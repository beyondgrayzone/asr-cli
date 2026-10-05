use asr::{ExecutionConfig, MultitalkerASR, Nemotron, NemotronMode};
use futures_util::{SinkExt, StreamExt};
use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};
use tokio::io::AsyncWriteExt;
use tokio::net::{TcpListener, TcpStream};
use tokio_tungstenite::accept_async;
use tokio_tungstenite::tungstenite::Message;

#[derive(Deserialize)]
struct ConfigMsg {
    mode: String, // "asr" or "multitalker"
    #[serde(default)]
    language: Option<String>, // "en" or "all"
    #[serde(default)]
    target_lang: Option<String>, // e.g. "tr-TR", "ja-JP", or "auto"
}

#[derive(Serialize)]
struct OutputMsg {
    text: String,
    speaker_id: Option<usize>,
    words: Option<Vec<WordTimestampMsg>>,
}

#[derive(Serialize)]
struct WordTimestampMsg {
    word: String,
    start_secs: f32,
    end_secs: f32,
}

async fn download_file(
    url: &str,
    dest: &Path,
) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    if dest.exists() && std::fs::metadata(dest)?.len() > 0 {
        return Ok(());
    }
    println!("Downloading {} to {}...", url, dest.display());
    let mut response = reqwest::get(url).await?;
    let mut file = tokio::fs::File::create(dest).await?;
    while let Some(chunk) = response.chunk().await? {
        file.write_all(&chunk).await?;
    }
    file.sync_all().await?;
    println!("Downloaded {}", dest.display());
    Ok(())
}

async fn ensure_models(
    mode: &str,
    language: &str,
) -> Result<PathBuf, Box<dyn std::error::Error + Send + Sync>> {
    let home = dirs::home_dir().unwrap_or_else(|| PathBuf::from("."));
    let asr_base_dir = home.join(".asr");
    tokio::fs::create_dir_all(&asr_base_dir).await?;

    if mode == "multitalker" {
        let multi_dir = asr_base_dir.join("multitalker");
        tokio::fs::create_dir_all(&multi_dir).await?;
        let files = vec![
            ("encoder.int8.onnx", "https://huggingface.co/smcleod/multitalker-parakeet-streaming-0.6b-v1-onnx-int8/resolve/main/encoder.int8.onnx"),
            ("decoder_joint.int8.onnx", "https://huggingface.co/smcleod/multitalker-parakeet-streaming-0.6b-v1-onnx-int8/resolve/main/decoder_joint.int8.onnx"),
            ("tokenizer.model", "https://huggingface.co/smcleod/multitalker-parakeet-streaming-0.6b-v1-onnx-int8/resolve/main/tokenizer.model"),
        ];
        for (filename, url) in files {
            download_file(url, &multi_dir.join(filename)).await?;
        }
        let sortformer_path = asr_base_dir.join("diar_streaming_sortformer_4spk-v2.1.onnx");
        download_file("https://huggingface.co/altunenes/parakeet-rs/resolve/main/diar_streaming_sortformer_4spk-v2.1.onnx", &sortformer_path).await?;
        Ok(multi_dir)
    } else {
        if language == "all" {
            let multi_dir = asr_base_dir.join("nemotron_multi");
            tokio::fs::create_dir_all(&multi_dir).await?;
            let files = vec![
                ("encoder.onnx", "https://huggingface.co/altunenes/parakeet-rs/resolve/main/nemotron-3.5-asr-streaming-0.6b-onnx/encoder.onnx"),
                ("encoder.onnx.data", "https://huggingface.co/altunenes/parakeet-rs/resolve/main/nemotron-3.5-asr-streaming-0.6b-onnx/encoder.onnx.data"),
                ("decoder_joint.onnx", "https://huggingface.co/altunenes/parakeet-rs/resolve/main/nemotron-3.5-asr-streaming-0.6b-onnx/decoder_joint.onnx"),
                ("tokenizer.model", "https://huggingface.co/altunenes/parakeet-rs/resolve/main/nemotron-3.5-asr-streaming-0.6b-onnx/tokenizer.model"),
            ];
            for (filename, url) in files {
                download_file(url, &multi_dir.join(filename)).await?;
            }
            Ok(multi_dir)
        } else {
            let nemotron_dir = asr_base_dir.join("nemotron_en");
            tokio::fs::create_dir_all(&nemotron_dir).await?;
            let files = vec![
                ("encoder.onnx", "https://huggingface.co/altunenes/parakeet-rs/resolve/main/nemotron-speech-streaming-en-0.6b/encoder.onnx"),
                ("encoder.onnx.data", "https://huggingface.co/altunenes/parakeet-rs/resolve/main/nemotron-speech-streaming-en-0.6b/encoder.onnx.data"),
                ("decoder_joint.onnx", "https://huggingface.co/altunenes/parakeet-rs/resolve/main/nemotron-speech-streaming-en-0.6b/decoder_joint.onnx"),
                ("tokenizer.model", "https://huggingface.co/altunenes/parakeet-rs/resolve/main/nemotron-speech-streaming-en-0.6b/tokenizer.model"),
            ];
            for (filename, url) in files {
                download_file(url, &nemotron_dir.join(filename)).await?;
            }
            Ok(nemotron_dir)
        }
    }
}

// ONNX intra-op thread count for inference.
//
// The encoder dominates a chunk's cost and parallelises well across cores, so
// this scales with the machine instead of being pinned to 1. Measured per-chunk
// cost for the 560ms Nemotron profile: 369ms at 1 thread, 240ms at 2, 235ms at
// 4, 189ms at 8 -- about 2x faster than single-threaded.
//
// Stays at 1 for small core counts so tiny containers, or a machine that is
// already busy, do not get oversubscribed into worse tail latency.
// Override with ASR_THREADS.
fn inference_threads() -> usize {
    if let Ok(v) = std::env::var("ASR_THREADS") {
        if let Ok(n) = v.parse::<usize>() {
            if n > 0 {
                return n;
            }
        }
    }
    let cores = std::thread::available_parallelism()
        .map(|n| n.get())
        .unwrap_or(1);
    if cores <= 2 {
        1
    } else {
        // Cap at 8: past that the thread-pool overhead cancels most of the gain
        // and the server stays responsive to other work.
        cores.min(8)
    }
}

async fn handle_connection(raw_stream: TcpStream) {
    let mut ws_stream = accept_async(raw_stream).await.expect("Failed to accept WS");
    println!("Client connected.");

    let config_msg = match ws_stream.next().await {
        Some(Ok(msg)) => msg.to_text().unwrap().to_string(),
        _ => return,
    };

    let config: ConfigMsg = match serde_json::from_str(&config_msg) {
        Ok(c) => c,
        Err(_) => return,
    };

    let selected_lang = config.language.clone().unwrap_or_else(|| "en".to_string());

    // Send loading status before potentially long IO/Initialization
    let _ = ws_stream.send(Message::Text("loading".into())).await;

    println!(
        "Requested mode: {}, language: {}. Starting model setup...",
        config.mode, selected_lang
    );

    let model_dir = match ensure_models(&config.mode, &selected_lang).await {
        Ok(d) => d,
        Err(e) => {
            let _ = ws_stream.send(Message::Text(format!("error: {}", e))).await;
            return;
        }
    };

    if config.mode == "multitalker" {
        let sortformer_path = dirs::home_dir()
            .unwrap()
            .join(".asr/diar_streaming_sortformer_4spk-v2.1.onnx");
        let model_dir_clone = model_dir.clone();

        // Initialize ONNX Sessions
        let model_res = tokio::task::spawn_blocking(move || {
            let cfg = ExecutionConfig::default()
                .with_intra_threads(inference_threads())
                .with_inter_threads(1);
            MultitalkerASR::from_pretrained(&model_dir_clone, &sortformer_path, Some(cfg))
        })
        .await
        .unwrap();

        let mut model = match model_res {
            Ok(m) => m,
            Err(e) => {
                let _ = ws_stream.send(Message::Text(format!("error: {}", e))).await;
                return;
            }
        };

        // Notify client that we are DONE loading and ready for audio
        println!("Multitalker model ready");
        ws_stream.send(Message::Text("ready".into())).await.unwrap();

        while let Some(Ok(msg)) = ws_stream.next().await {
            if msg.is_binary() {
                let floats: Vec<f32> = msg
                    .into_data()
                    .chunks_exact(4)
                    .map(|b| f32::from_le_bytes([b[0], b[1], b[2], b[3]]))
                    .collect();

                let current_model = model;
                let (m, results) = tokio::task::spawn_blocking(move || {
                    let mut im = current_model;
                    let r = im.transcribe_chunk(&floats);
                    (im, r)
                })
                .await
                .unwrap();
                model = m;

                if let Ok(transcripts) = results {
                    for r in transcripts {
                        let out = OutputMsg {
                            text: r.text,
                            speaker_id: Some(r.speaker_id),
                            words: Some(
                                r.words
                                    .into_iter()
                                    .map(|w| WordTimestampMsg {
                                        word: w.word,
                                        start_secs: w.start_secs,
                                        end_secs: w.end_secs,
                                    })
                                    .collect(),
                            ),
                        };
                        let _ = ws_stream
                            .send(Message::Text(serde_json::to_string(&out).unwrap()))
                            .await;
                    }
                }
            }
        }
    } else {
        let model_dir_clone = model_dir.clone();
        let target_lang_clone = config.target_lang.clone();
        let (tx, rx) = tokio::sync::oneshot::channel();

        std::thread::Builder::new()
            .name("onnx-loader".to_string())
            .stack_size(128 * 1024 * 1024)
            .spawn(move || {
                let local_exec_cfg = ExecutionConfig::default()
                    .with_intra_threads(inference_threads())
                    .with_inter_threads(1);
                let res = Nemotron::from_pretrained(&model_dir_clone, Some(local_exec_cfg));
                let _ = tx.send(res);
            })
            .expect("Failed to spawn loader thread");

        let mut model = match rx.await {
            Ok(Ok(mut m)) => {
                if let NemotronMode::Multilingual = m.mode() {
                    let lang = target_lang_clone.as_deref().unwrap_or("auto");
                    let _ = m.set_target_lang(lang);
                    println!("  [Status] Multilingual weights loaded.");
                } else {
                    println!("  [Status] English weights loaded.");
                }
                m
            }
            Ok(Err(e)) => {
                let _ = ws_stream.send(Message::Text(format!("error: {}", e))).await;
                return;
            }
            _ => return,
        };

        // Notify client that we are DONE loading and ready for audio
        println!("ASR model ready");
        ws_stream.send(Message::Text("ready".into())).await.unwrap();

        while let Some(Ok(msg)) = ws_stream.next().await {
            if msg.is_binary() {
                let floats: Vec<f32> = msg
                    .into_data()
                    .chunks_exact(4)
                    .map(|b| f32::from_le_bytes([b[0], b[1], b[2], b[3]]))
                    .collect();

                let current_model = model;
                let (m, result) = tokio::task::spawn_blocking(move || {
                    let mut im = current_model;
                    let r = im.transcribe_chunk(&floats);
                    (im, r)
                })
                .await
                .unwrap();
                model = m;

                if let Ok(text) = result {
                    if !text.is_empty() {
                        let out = OutputMsg {
                            text,
                            speaker_id: None,
                            words: None,
                        };
                        let _ = ws_stream
                            .send(Message::Text(serde_json::to_string(&out).unwrap()))
                            .await;
                    }
                }
            }
        }
    }
}

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = std::env::args().collect();
    let port = args
        .iter()
        .position(|arg| arg == "--port" || arg == "-p")
        .and_then(|i| args.get(i + 1).cloned())
        .or_else(|| std::env::var("ASR_PORT").ok())
        .unwrap_or_else(|| "9393".to_string());

    // ORT_*_NUM_THREADS are read by ONNX Runtime at session creation and would
    // otherwise override the per-session `intra_threads` below, so they are set
    // to the same scaled value rather than a hardcoded 1.
    let threads = inference_threads().to_string();
    std::env::set_var("ORT_ARENA_CFG", "cpu:0");
    std::env::set_var("ORT_INTRA_OP_NUM_THREADS", &threads);
    std::env::set_var("ORT_INTER_OP_NUM_THREADS", "1");
    std::env::set_var("OMP_NUM_THREADS", &threads);
    std::env::set_var("OMP_WAIT_POLICY", "PASSIVE");

    // `ort` self-initializes lazily on the first `Session::builder()` call
    // (inside `NemotronHandle::from_pretrained`), so no explicit global
    // `ort::init()` is needed here.
    let addr = format!("0.0.0.0:{}", port);
    tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()?
        .block_on(async {
            let listener = TcpListener::bind(&addr).await?;
            println!("Server listening on ws://{}", addr);
            while let Ok((stream, _)) = listener.accept().await {
                tokio::spawn(handle_connection(stream));
            }
            Ok(())
        })
}
