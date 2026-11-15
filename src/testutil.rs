use std::path::PathBuf;

pub fn temp_dir(tag: &str) -> PathBuf {
    let n = crate::crypto::random_u32();
    let base =
        std::env::var_os("TYPEDECK_TEST_TMP").map(PathBuf::from).unwrap_or_else(|| PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("target").join("test-tmp"));
    let d = base.join(format!("typedeck-test-{tag}-{}-{n:08x}", std::process::id()));
    std::fs::create_dir_all(&d).unwrap();
    d
}
