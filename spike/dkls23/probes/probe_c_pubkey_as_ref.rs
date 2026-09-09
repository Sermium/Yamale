//! C: the same key, reached with as_ref() instead.
//!
//! Run alongside B rather than after it: whichever compiles names the return
//! type, and if both fail the type is something like EncodedPoint that needs
//! as_bytes(). Two probes answer in one run what one probe answers in two.
mod common;

use common::shared::gen_keyshares;

#[tokio::main]
async fn main() {
    let shares = gen_keyshares(2, 3).await;
    let bytes = shares[0].public_key().to_bytes();
    let _: &[u8] = bytes.as_ref();
}
