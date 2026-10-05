# Most ASR software is built using latest linux kernel, so it wont work on my other laptop which is running on older glibc. Hence asr-cli was created

> This repo is more cleaned version of my private repo where the actual dev is happening
> This asr was built for Linux systems with glibc version < 2.38. 
> While it supports Multi lingual, I hav mostly focused on English

## Note that this is fork of `https://github.com/altunenes/parakeet-rs` which does not have either server or client. This repo fixes that

## Run server 

cargo run 

## Run client 
```
cd client 

go run . -start -mode asr -lang en -stdout
```

# Once thr above is done, the models should get automatically downloaded to `~./.asr` folder
> ~/.asr/nemotron_en


