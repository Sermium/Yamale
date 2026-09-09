//! A: do the example helpers resolve, and are their signatures what the source said?
//!
//! Compile-only. Nothing here runs — cargo build type-checks it and that is the
//! whole question. If this fails, every probe after it fails for the same
//! reason and the others tell you nothing.
mod common;

use common::shared::{gen_keyshares, setup_dsg};

#[tokio::main]
async fn main() {
    let shares = gen_keyshares(2, 3).await;
    let _setups = setup_dsg(&shares[0..2], "m");
}
