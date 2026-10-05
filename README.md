# asr-cli

Most ASR software is built against the latest Linux kernel and modern glibc, which means it will not run on my other laptop that is stuck on an older glibc. This project exists to solve that problem.

> This repo is a cleaned-up version of my private repository, where the actual development happens.

> Built specifically for Linux systems with glibc version < 2.38.

> While it supports multiple languages, the focus has mostly been on English.

## About

This is a fork of [Parakeet-rs](https://github.com/altunenes/parakeet-rs), which ships neither a server nor a client. This repo adds both.

## Demo

https://github.com/user-attachments/assets/ad13c9fb-5cf0-485d-b9ed-615e3d745bae

## Run the server

```bash
cargo run
```

## Run the client (prints to stdout)

```bash
cd client
go run . -start -mode asr -lang en -stdout
```

## Models

Once the server and client are running, models are downloaded automatically to the `~/.asr` directory.

```text
~/.asr/nemotron_en
```
