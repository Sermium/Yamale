# Guides

Task-shaped walkthroughs. Each states what you need before starting and what you
should have at the end, and every command has been run against a real node
before publishing.

| Guide | What it gets you |
| --- | --- |
| [What Yamale is](concepts.md) | The ideas the rest of the documentation assumes. No commands. |
| [Run a local chain](local-devnet.md) | A single-node network on your machine, with the API open. |
| [Set up a treasury](treasury.md) | Shared funds with roles, spending limits and a vesting schedule. |
| [What governance can and cannot change](constitution.md) | The line between an ordinary parameter and an invariant, and how one is amended. |
| [The key ceremony](key-ceremony.md) | The room, the paper and the 3-of-5 that ends up holding every seized asset. |
| [Enrolling a country](country-enrolment.md) | A country's offices, their M-of-N accounts and the powers they hold inside their own borders. |
| [Appointing a foundation administrator](foundation-administrators.md) | The one account that can move a customer out from under the authority investigating them, and why only a governance vote can grant it. |
| [Netting and settlement](settlement.md) | What the chain settles between institutions, and what to do when a window does not clear. |
| [Threshold accounts](mpc.md) | A consumer key that exists in three shares and nowhere else, and the password reset that does not move the address. |
| [The custodian](custodian.md) | The service holding the operator's share — what it refuses, and the longer list of what it does not do. |
| [Accounts without a wallet](accounts.md) | The design the two above were built from, and the parts of it that are still only a design. |

For exhaustive detail on any message, query, parameter or error code, see the
[reference](../reference/), which is generated from the code and cannot drift
from it.
