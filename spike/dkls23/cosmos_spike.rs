//! Does a DKLs23 signature work on this chain?
//!
//! This is the only question worth asking first, and it is the same principle
//! `tools/mpc pay` was written for: everything above the signature can be
//! satisfied by a library that is subtly wrong — a signature over the wrong
//! bytes, a public key encoded the way this codebase does not expect, an S
//! value in the upper half of the curve order — and all of those look identical
//! to a caller checking that a function returned no error.
//!
//! So this prints the three values that decide it, and `verify_test.go` checks
//! them against the chain's own derivation rather than against a claim.
//!
//! # Why it lives inside the library's examples directory
//!
//! Copied into the checked-out `silence-laboratories/dkls23` tree by
//! `.github/workflows/dkls-spike.yml` rather than built as a crate of our own.
//! Two reasons, both about not guessing: it resolves the same dependency
//! versions the library itself pins, so there is no second `k256` in the graph
//! and no type mismatch to debug; and it can use `examples/common.rs`, which is
//! code already known to compile against this exact revision.
//!
//! The digest is therefore `[1u8; 32]` — `setup_dsg` hardcodes
//! `.with_hash([1; 32])`, and reaching past that to sign a real Cosmos SignDoc
//! means building the sign setup by hand. That is worth doing, and it is step
//! two: it needs the sidecar's own setup construction.

mod common;

// GroupEncoding is what `to_bytes()` on the public key comes from, and it has
// to be in scope or the method does not resolve at all. Its absence is what
// failed the first two probes — identically, which is what said the problem was
// the method rather than the accessor after it.
use k256::elliptic_curve::group::GroupEncoding;

use common::shared::{gen_keyshares, setup_dsg};
use rand::Rng;
use rand_chacha::ChaCha20Rng;
use rand_core::SeedableRng;
use sl_dkls23::sign;
use sl_mpc_mate::coord::SimpleMessageRelay;
use tokio::task::JoinSet;

/// The digest `setup_dsg` signs. Named because the Go side has to assert
/// against the same bytes, and a mismatch would look like a bad signature
/// rather than like two different messages.
const DIGEST: [u8; 32] = [1u8; 32];

#[tokio::main]
async fn main() {
    // 2 of 3, which is this chain's arrangement: device, custodian, recovery.
    let shares = gen_keyshares(2, 3).await;

    // Every share agrees on the joint key and each derives it locally. Taking
    // it from one share is fine here precisely because they must all match; the
    // Go side re-derives the address from these bytes rather than being told it.
    let pubkey = shares[0].public_key().to_bytes();

    // Two parties sign. Three would not be safer, it would just be three shares
    // in one process — which is what the whole arrangement exists to avoid, and
    // is why this runs them as separate tasks over a relay even though one
    // process holds all of them here.
    let coord = SimpleMessageRelay::new();
    let mut rnd = ChaCha20Rng::from_entropy();
    let mut parties = JoinSet::new();
    for setup in setup_dsg(&shares[0..2], "m") {
        let relay = coord.connect();
        let seed = rnd.gen();
        parties.spawn(async move { sign::run(setup, seed, relay).await });
    }

    let mut result = None;
    while let Some(joined) = parties.join_next().await {
        let signed = joined.expect("signing task panicked").expect("signing failed");
        result = Some(signed);
    }
    let (signature, recid) = result.expect("no party produced a signature");

    // JSON on stdout so the workflow can hand it straight to Go.
    println!("{{");
    println!("  \"protocol\": \"dkls23\",");
    println!("  \"library\": \"sl-dkls23\",");
    println!("  \"threshold\": \"2-of-3\",");
    println!("  \"chain_path\": \"m\",");
    println!("  \"pubkey_sec1\": \"{}\",", hex::encode(pubkey));
    println!("  \"digest\": \"{}\",", hex::encode(DIGEST));
    println!("  \"signature_rs\": \"{}\",", hex::encode(signature.to_bytes()));
    println!("  \"recovery_id\": {}", recid.to_byte());
    println!("}}");
}
