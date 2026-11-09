use std::{
    ffi::c_void,
    ptr,
    time::{Instant, SystemTime, UNIX_EPOCH},
};
mod rotary {
    include!(concat!(
        env!("MISTRALRS_SOURCE"),
        "/mistralrs-quant/src/rotary/ffi.rs"
    ));
}
#[link(name = "cudart")]
unsafe extern "C" {
    fn cudaSetDevice(device: i32) -> i32;
    fn cudaStreamCreate(stream: *mut *mut c_void) -> i32;
    fn cudaStreamSynchronize(stream: *mut c_void) -> i32;
    fn cudaStreamDestroy(stream: *mut c_void) -> i32;
    fn cudaMalloc(pointer: *mut *mut c_void, bytes: usize) -> i32;
    fn cudaFree(pointer: *mut c_void) -> i32;
    fn cudaMemcpy(dest: *mut c_void, source: *const c_void, bytes: usize, kind: i32) -> i32;
    fn cudaGetLastError() -> i32;
}
fn check(code: i32) {
    assert_eq!(code, 0, "CUDA error {code}");
}
unsafe fn upload<T>(values: &[T]) -> *mut c_void {
    let mut pointer = ptr::null_mut();
    check(cudaMalloc(&mut pointer, size_of_val(values)));
    check(cudaMemcpy(
        pointer,
        values.as_ptr().cast(),
        size_of_val(values),
        1,
    ));
    pointer
}
fn main() {
    let operation = format!(
        "rotary-{:x}",
        SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_nanos()
    );
    let started = Instant::now();
    println!("operation={operation} phase=start");
    unsafe {
        check(cudaSetDevice(0));
        let mut stream = ptr::null_mut();
        check(cudaStreamCreate(&mut stream));
        assert!(
            (stream as usize) > u32::MAX as usize,
            "test needs a stream handle exceeding 32 bits"
        );
        let query = upload(&[1_f32, 2_f32]);
        let cos = upload(&[1_f32]);
        let sin = upload(&[0_f32]);
        let positions = upload(&[0_u32]);
        println!(
            "operation={operation} phase=kernel stream_bits={} c_long_bits={}",
            size_of::<*mut c_void>() * 8,
            size_of::<std::ffi::c_long>() * 8
        );
        rotary::rotary_embedding_positions(
            query,
            ptr::null(),
            cos,
            sin,
            positions,
            1,
            2,
            1_i64,
            1,
            1,
            1,
            0,
            2_i64,
            0_i64,
            2,
            stream as i64,
        );
        check(cudaGetLastError());
        rotary::rotary_embedding(
            query,
            ptr::null(),
            cos,
            sin,
            1,
            2,
            1_i64,
            1,
            1,
            0,
            2_i64,
            0_i64,
            2,
            stream as i64,
        );
        check(cudaGetLastError());
        check(cudaStreamSynchronize(stream));
        let mut output = [0_f32; 2];
        check(cudaMemcpy(
            output.as_mut_ptr().cast(),
            query,
            size_of_val(&output),
            2,
        ));
        assert_eq!(output, [1_f32, 2_f32]);
        for pointer in [query, cos, sin, positions] {
            check(cudaFree(pointer));
        }
        check(cudaStreamDestroy(stream));
        println!(
            "operation={operation} phase=complete output={output:?} duration_ms={}",
            started.elapsed().as_millis()
        );
    }
}
