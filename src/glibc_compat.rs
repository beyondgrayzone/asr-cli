// glibc 2.38+ compatibility layer (linux-gnu only).
//
// `ort-sys` ships a prebuilt `libonnxruntime.a` built against a newer
// glibc, where the `strtol` family was renamed to the C23 `__isoc23_*`
// variants. On an older glibc those three symbols are absent, so a static
// link fails with "undefined symbol: __isoc23_strtol".
//
// The `load-dynamic` feature looks like the escape hatch, but it is a trap
// when no `libonnxruntime.so` is installed: `ort` calls `dlopen` at
// runtime, that fails, and the error-reporting path re-enters `ort::api()`
// while its `OnceLock` is still initializing -- a self-deadlock on a
// `std::sync::Once` futex that hangs the process forever with no output.
// (The server's `onnx-loader` thread ends up parked in `futex_wait` at 0%
// CPU.)
//
// So we define the three symbols ourselves and forward them to the host
// glibc's `strtol` family. Argument lists are identical and on x86_64
// `long` and `long long` are both 64-bit, so the return value passes back
// in RAX unchanged. The only behavioural difference is losing C23 locale
// digit-grouping, which ONNX Runtime never uses for these numeric parses.

#![cfg(all(target_os = "linux", target_env = "gnu"))]

use core::ffi::c_char;

unsafe extern "C" {
    fn strtol(nptr: *const c_char, endptr: *mut *mut c_char, base: i32) -> isize;
    fn strtoll(nptr: *const c_char, endptr: *mut *mut c_char, base: i32) -> i64;
    fn strtoull(nptr: *const c_char, endptr: *mut *mut c_char, base: i32) -> u64;
}

// `__isoc23_strtol` -> `strtol`
//
// # Safety
// Standard C string-parsing contract; pointers must be valid for the same
// access pattern `strtol` requires.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn __isoc23_strtol(
    nptr: *const c_char,
    endptr: *mut *mut c_char,
    base: i32,
) -> isize {
    unsafe { strtol(nptr, endptr, base) }
}

// `__isoc23_strtoll` -> `strtoll`
//
// # Safety
// Standard C string-parsing contract; pointers must be valid for the same
// access pattern `strtoll` requires.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn __isoc23_strtoll(
    nptr: *const c_char,
    endptr: *mut *mut c_char,
    base: i32,
) -> i64 {
    unsafe { strtoll(nptr, endptr, base) }
}

// `__isoc23_strtoull` -> `strtoull`
//
// # Safety
// Standard C string-parsing contract; pointers must be valid for the same
// access pattern `strtoull` requires.
#[unsafe(no_mangle)]
pub unsafe extern "C" fn __isoc23_strtoull(
    nptr: *const c_char,
    endptr: *mut *mut c_char,
    base: i32,
) -> u64 {
    unsafe { strtoull(nptr, endptr, base) }
}
