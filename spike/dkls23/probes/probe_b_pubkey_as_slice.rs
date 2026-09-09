//! B: is the joint public key reachable as a byte slice via as_slice()?
//!
//! keygen.rs prints `keyshare.public_key().to_bytes()`, so to_bytes() exists.
//! What it RETURNS is the open question, and it decides how the sidecar hands
//! a key to Go. as_slice() works for [u8; N], Vec<u8> and GenericArray.
mod common;

use common::shared::gen_keyshares;

#[tokio::main]
async fn main() {
    let shares = gen_keyshares(2, 3).await;
    let bytes = shares[0].public_key().to_bytes();
    let _: &[u8] = bytes.as_slice();
}
