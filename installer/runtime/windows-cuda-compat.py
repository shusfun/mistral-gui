"""Apply narrowly scoped upstream v0.9.5 Windows CUDA compatibility fixes."""
from pathlib import Path
import sys

root = Path(sys.argv[1])
rotary_ffi = root / "mistralrs-quant/src/rotary/ffi.rs"
rotary_impl = root / "mistralrs-quant/src/rotary/mod.rs"
for path, old_import, new_import, arguments in (
    (rotary_ffi, "use core::ffi::{c_int, c_long, c_void};", "use core::ffi::{c_int, c_void};", 8),
    (rotary_impl, "use std::ffi::{c_int, c_long};", "use std::ffi::c_int;", 5),
):
    source = path.read_text(encoding="utf-8")
    if "c_long" in source:
        if old_import not in source or source.count("c_long") != arguments + 1:
            raise SystemExit("Unsupported upstream rotary FFI; review the Windows ABI patch")
        # The C++ entry points use int64_t, including the CUDA stream handle.
        # Windows uses LLP64: c_long is only 32 bits even in a 64-bit process.
        path.write_text(source.replace(old_import, new_import).replace("c_long", "i64"), encoding="utf-8")
        print("Windows CUDA compatibility: match rotary int64_t arguments without truncating stream handles")

header = root / "mistralrs-quant/kernels/marlin/marlin_kernel.cuh"
source = header.read_text(encoding="utf-8")
if "#include <string>" not in source:
    anchor = "#include <iostream>"
    if anchor not in source:
        raise SystemExit("Unsupported upstream Marlin header; review the Windows compatibility patch")
    header.write_text(source.replace(anchor, anchor + "\n#include <string>", 1), encoding="utf-8")
    print("Windows CUDA compatibility: added explicit <string> include to Marlin")
else:
    print("Upstream already includes <string>; no compatibility change required")

graph = root / "mistralrs-core/src/pipeline/cuda_graph.rs"
source = graph.read_text(encoding="utf-8")
signature = "pub(crate) fn prepare_fa3_decode_schedules("
guard = '#[cfg(target_family = "unix")]\n' + signature
if guard not in source:
    if source.count(signature) != 1:
        raise SystemExit("Unsupported upstream FA3 schedule function; review the Windows patch")
    fallback = '''#[cfg(not(target_family = "unix"))]
pub(crate) fn prepare_fa3_decode_schedules(
    _metadata: &PagedAttentionInputMetadata,
) -> candle_core::Result<()> {
    // FA3 state is unavailable on this target, matching make_fa3_decode_state.
    Ok(())
}

'''
    graph.write_text(source.replace(signature, fallback + guard, 1), encoding="utf-8")
    print("Windows CUDA compatibility: guard Unix-only FA3 schedule preparation")

layers = root / "mistralrs-core/src/paged_attention/layers/mod.rs"
source = layers.read_text(encoding="utf-8")
if "pub fn write_cache(" not in source:
    anchor = "        pub fn fp8_attention_scales(&self) -> Fp8AttentionScales {"
    if anchor not in source:
        raise SystemExit("Unsupported upstream PagedAttention fallback; review the Windows patch")
    fallback = '''        pub fn write_cache(
            &self,
            _key: &Tensor,
            _value: &Tensor,
            _key_cache: &mut Tensor,
            _value_cache: &mut Tensor,
            _input_metadata: &PagedAttentionInputMetadata,
        ) -> Result<()> {
            candle_core::bail!("Paged cache writes are unavailable on this target.");
        }

'''
    layers.write_text(source.replace(anchor, fallback + anchor, 1), encoding="utf-8")
    print("Windows CUDA compatibility: add explicit unsupported paged-cache write fallback")
