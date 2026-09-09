//! E: the two accessors the Go side depends on.
//!
//! 64 bytes of R||S is what a Cosmos signature is, and the recovery id is not
//! needed by the chain but is printed so the spike output is complete. If this
//! fails while D passes, the protocol is right and only the encoding is wrong.
mod common;

use common::shared::{gen_keyshares, setup_dsg};
use rand::Rng;
use rand_chacha::ChaCha20Rng;
use rand_core::SeedableRng;
use sl_dkls23::sign;
use sl_mpc_mate::coord::SimpleMessageRelay;

#[tokio::main]
async fn main() {
    let shares = gen_keyshares(2, 3).await;
    let coord = SimpleMessageRelay::new();
    let mut rnd = ChaCha20Rng::from_entropy();

    for setup in setup_dsg(&shares[0..2], "m") {
        let (signature, recid) = sign::run(setup, rnd.gen(), coord.connect())
            .await
            .unwrap();
        let _: &[u8] = signature.to_bytes().as_slice();
        let _: u8 = recid.to_byte();
    }
}
