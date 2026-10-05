// Build script for the `asr` crate.
//
// Sole job: provide the `asr_allow_load_dynamic` cfg used to gate the
// dangerous `load-dynamic` feature. See the `load-dynamic` entry in
// Cargo.toml for why enabling it by default is unsafe.

fn main() {
    println!("cargo::rerun-if-env-changed=ASR_ALLOW_LOAD_DYNAMIC");
    println!("cargo::rustc-check-cfg=cfg(asr_allow_load_dynamic)");

    // Opt-in escape hatch. Deliberately an environment variable rather than a
    // second cargo feature: a feature would compose with `load-dynamic`
    // transitively and defeat the check, whereas this requires the person
    // building to consciously set a variable in the same command.
    if std::env::var_os("ASR_ALLOW_LOAD_DYNAMIC").is_some() {
        println!("cargo::rustc-cfg=asr_allow_load_dynamic");
    }
}