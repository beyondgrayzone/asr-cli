# asr-server

Real-time speech recognition server: NVIDIA **Parakeet / Nemotron** models via ONNX Runtime, exposed over a
WebSocket API.

- `src/`  the `asr` Rust library (ONNX wrappers, mel front-end, RNNT decoding, diarisation)
- `server/`  the `asr-server` binary: model download + WebSocket server
- `client/`  Go microphone client that streams to the server

## Docker (recommended)

### Create a volume to store models

```
docker volume create asr_data
```

### Build

```
BUILDKIT_PROGRESS=plain docker build -f Dockerfile -t asr_server:0.0.1 .
```

### Run

```
docker run --name asr_server -it -p 9393:9393 -v asr_data:/home/ubuntu/.asr asr_server:0.0.1
```

`asr_data` is mounted at `/home/ubuntu/.asr`, so downloaded models survive container restarts and rebuilds.
The image is CPU-only; it ships `libgomp1` and `libssl3` for ONNX Runtime.

## Build from source

```
cd server
cargo build --release
./target/release/asr-server
```

### If linking fails with `undefined symbol: __isoc23_strtol`

`ort-sys` downloads a prebuilt `libonnxruntime` that requires **glibc ≥ 2.38** (Ubuntu 24.04+). On older
systems (Ubuntu 22.04 ships glibc 2.35) the link step reports three missing symbols:

```
rust-lld: error: undefined symbol: __isoc23_strtol
rust-lld: error: undefined symbol: __isoc23_strtoll
rust-lld: error: undefined symbol: __isoc23_strtoull
```

glibc 2.38 renamed the `strtol` family to the C23 `__isoc23_*` variants, and the prebuilt binary
references the new names. This is already handled for you: `src/glibc_compat.rs` defines the three
symbols and forwards them to the host glibc's `strtol`/`strtoll`/`strtoull`. The `long` and `long long`
return values are ABI-identical on x86_64, and the only behavioural difference (C23 locale
digit-grouping) is not used by ONNX Runtime. A plain `cargo build` works on both old and new glibc, and
the module is a no-op on non-`linux-gnu` targets.

> **Do not use `--features load-dynamic` to work around this.** It only helps if a matching
> `libonnxruntime.so` is already installed system-wide. When none is, `ort` falls back to `dlopen` at
> runtime, that call fails, and the error-reporting path re-enters `ort::api()` while its `OnceLock` is
> still initializing  a self-deadlock on a `std::sync::Once` futex. The process hangs forever with no
> error output and the loader thread sits at 0% CPU in `futex_wait`. The build appears to succeed, so
> this is easy to mistake for a slow model load.
>
> A plain `cargo run --release` is the correct command. If you have a script or alias that passes
> `--features load-dynamic`, drop the flag: `cargo run --release` works on both old and new glibc.

### If you really need `load-dynamic`

The feature is gated at compile time so the silent hang above cannot happen by accident. Setting
`load-dynamic` without opting in is a build error:

```
error: asr: `load-dynamic` needs ASR_ALLOW_LOAD_DYNAMIC=1; see Cargo.toml. Otherwise build plain.
```

To override, set `ASR_ALLOW_LOAD_DYNAMIC=1` in the same command  but only do that if you have
confirmed a matching `libonnxruntime.so` is actually installed system-wide:

```
ASR_ALLOW_LOAD_DYNAMIC=1 ORT_DYLIB_PATH=/usr/lib/libonnxruntime.so cargo run --release --features load-dynamic
```

Note that `ASR_ALLOW_LOAD_DYNAMIC` is checked at *build* time, so changing it later has no effect on
an already-built binary. If the server hangs, check how the current binary was built:

```
nm target/debug/asr-server | grep -c isoc23   # 3 = good, 0 = built with load-dynamic
```

## Run

```
asr-server [--port <port>]
```

| Argument | Default | Description |
| --- | --- | --- |
| `--port` / `-p` | `9393` | TCP port to bind on `0.0.0.0` |
| `ASR_PORT` (env) | `9393` | Used when `--port` is not given |

```
./asr-server --port 9393
ASR_PORT=8080 ./asr-server
```

The server tunes ONNX Runtime for low-latency single-threaded streaming (`ORT_INTRA_OP_NUM_THREADS=1`,
`ORT_INTER_OP_NUM_THREADS=1`, `ORT_ARENA_CFG=cpu:0`, `OMP_WAIT_POLICY=PASSIVE`).

## Models

Models are downloaded **on first connection**, not at startup. They are cached in `~/.asr/` and reused
afterwards (a file is skipped if it already exists and is non-empty).

| Mode / language | Directory | Files |
| --- | --- | --- |
| `multitalker` | `~/.asr/multitalker` | `encoder.int8.onnx`, `decoder_joint.int8.onnx`, `tokenizer.model` |
| `multitalker` | `~/.asr/` | `diar_streaming_sortformer_4spk-v2.1.onnx` |
| `asr` + `language: "en"` | `~/.asr/nemotron_en` | `encoder.onnx` + `encoder.onnx.data`, `decoder_joint.onnx`, `tokenizer.model` |
| `asr` + `language: "all"` | `~/.asr/nemotron_multi` | `encoder.onnx` + `encoder.onnx.data`, `decoder_joint.onnx`, `tokenizer.model` |

Sources:

- `multitalker` → [`smcleod/multitalker-parakeet-streaming-0.6b-v1-onnx-int8`](https://huggingface.co/smcleod/multitalker-parakeet-streaming-0.6b-v1-onnx-int8)
- Sortformer + Nemotron → [`altunenes/parakeet-rs`](https://huggingface.co/altunenes/parakeet-rs)

Downloading several GB can take a while. The client prints a status line while this is in progress  let it
finish before assuming the connection failed.

### Which model gets picked

`language` selects the **weights**, `target_lang` selects the **prompt index** inside the multilingual model:

- `language: "en"`  English-specialist Nemotron 0.6B (vocab 1024, no language conditioning).
  `target_lang` is ignored.
- `language: "all"`  multilingual Nemotron 3.5 0.6B (vocab ~13k, has a `prompt_index` input).
  `target_lang` accepts any key from the prompt dictionary, or `auto` (the default) to let the model
  detect the language itself.

## WebSocket protocol

Connect to `ws://<host>:9393`. Audio must be **16 kHz, mono, `f32` little-endian, raw** (no WAV header).

### 1. Client → server: configuration (text frame, first message)

```json
{ "mode": "asr", "language": "en", "target_lang": "tr-TR" }
```

| Field | Required | Values |
| --- | --- | --- |
| `mode` | yes | `asr` or `multitalker` |
| `language` | no | `en` (default) or `all` |
| `target_lang` | no | e.g. `tr-TR`, `ja-JP`, `auto`. Only used with `language: "all"` |

`mode` is matched literally  anything other than `multitalker` falls through to the Nemotron/`asr` path.

### 2. Server → client: status

| Message | Meaning |
| --- | --- |
| `loading` | Download/load in progress. Send nothing yet; binary frames sent now may be dropped while the model loads. |
| `ready` | Model loaded. Start streaming audio. |
| `error: <message>` | Download or load failed. The server then closes the connection. |

### 3. Client → server: audio (binary frames)

Each binary frame is a packed array of `f32` LE samples. Chunk size is up to you  the model buffers
internally and only emits text once it has a full encoder chunk (~560 ms for `asr`, ~1.12 s for
`multitalker`).

### 4. Server → client: transcripts (text frames, JSON)

```json
{ "text": "hello world", "speaker_id": 0, "words": [ { "word": "hello", "start_secs": 0.08, "end_secs": 0.24 } ] }
```

| Field | `asr` mode | `multitalker` mode |
| --- | --- | --- |
| `text` | emitted text delta for this chunk | emitted text delta for this speaker |
| `speaker_id` | `null` | speaker index (0–3) |
| `words` | `null` | word-level timestamps for this delta |

In `asr` mode empty deltas are suppressed. In `multitalker` mode you get one message per active speaker
per chunk, and `speaker_id` marks who is talking.

### Protocol notes

- One model instance is created **per connection**, so each connection pays full model load cost and its own
  memory. Reuse a single connection for a session.
- There is no explicit end-of-stream message; closing the socket ends the session.
- The server does not reset decoder state between chunks on its own  treat a new `ConfigMsg`/connection as a
  fresh stream.

## Feature flags

Default build is CPU-only with ONNX Runtime's default providers. Accelerator backends are compiled in only
when their `asr` feature is enabled:

```
cargo build --release --features cuda       # NVIDIA CUDA (falls back to CPU)
cargo build --release --features tensorrt   # NVIDIA TensorRT
cargo build --release --features coreml     # Apple CoreML / ANE
cargo build --release --features directml   # Windows DirectML
cargo build --release --features migraphx
cargo build --release --features openvino
cargo build --release --features webgpu     # experimental
cargo build --release --features nnapi      # Android
```

The `server` binary always uses `ExecutionProvider::Cpu` with 1 intra / 1 inter thread. To pick a
different provider you construct your own `ExecutionConfig` when using the `asr` library directly.

Optional model features in the library: `multitalker` (Sortformer + speaker kernels, enabled by default in
`server/`), `sortformer`, `cohere`.

## Tests

```
cargo test --lib          # unit tests, no model files required
```

## Library usage

```rust
use asr::{ExecutionConfig, Nemotron, NemotronMode};

let mut asr = Nemotron::from_pretrained(
    "~/.asr/nemotron_multi",
    Some(ExecutionConfig::default().with_intra_threads(1)),
)?;

if asr.mode() == NemotronMode::Multilingual {
    asr.set_target_lang("tr-TR")?;   // or "auto"
}

// Streaming: call repeatedly with ~20-100ms of 16kHz mono audio
let delta = asr.transcribe_chunk(&audio_chunk)?;

// Offline
let full = asr.transcribe_audio(&audio)?;
```

For several concurrent streams sharing one loaded model, use `NemotronHandle::from_pretrained` once and then
`Nemotron::from_shared(&handle)` per stream  the ONNX session is loaded and reference counted, and each
stream keeps independent decoder state.