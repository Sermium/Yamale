package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txtypes "github.com/cosmos/cosmos-sdk/types/tx"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	"yamale/blockchain/mpc"
	mpccosmos "yamale/blockchain/mpc/cosmos"
)

// runPay builds, signs and broadcasts a real transaction from a threshold
// account.
//
// This is the command the rest of the tool exists to reach. A library can be
// subtly wrong in ways every unit test agrees with — a signature over the wrong
// bytes, a public key encoded the way this codebase does not expect, an S value
// in the upper half of the curve — and all of them look identical to a caller
// checking that Sign returned no error. A chain either accepts the transaction
// or it does not.
//
// Two shares sign. The third is not read, is not needed, and if it happens to
// be sitting in the same directory that is a fact about this rehearsal and not
// about the design.
func runPay(args []string) error {
	fs := flag.NewFlagSet("pay", flag.ExitOnError)
	dir := fs.String("shares", "", "directory holding the share files")
	var explicit shareList
	fs.Var(&explicit, "share", "a specific share file; repeat (overrides --shares)")
	to := fs.String("to", "", "recipient address")
	amount := fs.String("amount", "", "amount, e.g. 250000uyml")
	node := fs.String("node", "http://localhost:26657", "CometBFT RPC endpoint")
	chainID := fs.String("chain-id", "yamale-devnet-2", "chain id")
	memo := fs.String("memo", "signed by two shares, held apart", "transaction memo")
	feeAmount := fs.String("fee", "8000uyml", "fee")
	gasLimit := fs.Uint64("gas", 200000, "gas limit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	shares, err := payShares(*dir, explicit)
	if err != nil {
		return err
	}
	if len(shares) != mpc.Threshold+1 {
		return fmt.Errorf(
			"pay signs with exactly %d shares and was given %d; more is not safer, it is just more shares in one place",
			mpc.Threshold+1, len(shares))
	}

	pub, err := anyShare(shares).PublicKey()
	if err != nil {
		return err
	}
	pk, err := mpccosmos.CosmosPubKey(pub)
	if err != nil {
		return err
	}
	from, err := mpccosmos.Address(pub)
	if err != nil {
		return err
	}

	coins, err := sdk.ParseCoinsNormalized(*amount)
	if err != nil {
		return fmt.Errorf("--amount: %w", err)
	}
	fees, err := sdk.ParseCoinsNormalized(*feeAmount)
	if err != nil {
		return fmt.Errorf("--fee: %w", err)
	}
	if strings.TrimSpace(*to) == "" {
		return fmt.Errorf("--to is required")
	}

	registry := codectypes.NewInterfaceRegistry()
	authtypes.RegisterInterfaces(registry)
	banktypes.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	accNum, seq, err := accountOf(*node, cdc, from)
	if err != nil {
		return fmt.Errorf("reading %s from the chain: %w", from, err)
	}
	fmt.Fprintf(os.Stderr, "account %s  number %d  sequence %d\n", from, accNum, seq)

	msg := &banktypes.MsgSend{FromAddress: from, ToAddress: *to, Amount: coins}
	msgAny, err := codectypes.NewAnyWithValue(msg)
	if err != nil {
		return err
	}
	body := &txtypes.TxBody{Messages: []*codectypes.Any{msgAny}, Memo: *memo}
	bodyBytes, err := cdc.Marshal(body)
	if err != nil {
		return err
	}

	pkAny, err := codectypes.NewAnyWithValue(pk)
	if err != nil {
		return err
	}
	authInfo := &txtypes.AuthInfo{
		SignerInfos: []*txtypes.SignerInfo{{
			PublicKey: pkAny,
			ModeInfo: &txtypes.ModeInfo{Sum: &txtypes.ModeInfo_Single_{
				Single: &txtypes.ModeInfo_Single{Mode: signingtypes.SignMode_SIGN_MODE_DIRECT},
			}},
			Sequence: seq,
		}},
		Fee: &txtypes.Fee{Amount: fees, GasLimit: *gasLimit},
	}
	authInfoBytes, err := cdc.Marshal(authInfo)
	if err != nil {
		return err
	}

	signDoc := &txtypes.SignDoc{
		BodyBytes:     bodyBytes,
		AuthInfoBytes: authInfoBytes,
		ChainId:       *chainID,
		AccountNumber: accNum,
	}
	signBytes, err := cdc.Marshal(signDoc)
	if err != nil {
		return err
	}

	// The chain's verifier hashes what it is given, so the digest signed here
	// is sha256 of the sign bytes and the signature is checked against the sign
	// bytes themselves. Signing the hash of the hash produces something that
	// verifies against nothing, which is a mistake this codebase has already
	// made once, in a test, on 2026-08-30.
	digest := sha256sum(signBytes)

	fmt.Fprintf(os.Stderr, "signing with %s\n", strings.Join(rolesOf(shares), " + "))
	start := time.Now()
	sig, _, err := mpc.Sign(digest, shares)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "signature in %s\n", time.Since(start).Round(time.Millisecond))

	// Checked here, before broadcast, with the chain's own verifier. A refusal
	// on the node tells you a transaction failed; this tells you why, while the
	// bytes that produced it are still in hand.
	if !pk.VerifySignature(signBytes, sig) {
		return fmt.Errorf("the signature does not verify against its own sign bytes; not broadcasting")
	}

	raw := &txtypes.TxRaw{
		BodyBytes:     bodyBytes,
		AuthInfoBytes: authInfoBytes,
		Signatures:    [][]byte{sig},
	}
	txBytes, err := cdc.Marshal(raw)
	if err != nil {
		return err
	}

	hash, code, log, err := broadcast(*node, txBytes)
	if err != nil {
		return err
	}
	fmt.Println(hash)
	if code != 0 {
		return fmt.Errorf("the chain refused it: code %d, %s", code, log)
	}
	fmt.Fprintf(os.Stderr, "accepted: %s\n", hash)
	return nil
}

// payShares resolves which two shares to sign with.
//
// Defaults to device + custodian, because that is the ordinary path a person
// takes every day; recovery is for the day something has gone wrong and naming
// it should be deliberate.
func payShares(dir string, explicit []string) (map[string]mpc.Share, error) {
	if len(explicit) > 0 {
		return readShares(explicit)
	}
	if strings.TrimSpace(dir) == "" {
		return nil, fmt.Errorf("give --shares DIR or two --share files")
	}
	var paths []string
	for _, role := range []string{mpc.RoleDevice, mpc.RoleCustodian} {
		p := filepath.Join(dir, role+".share.json")
		if _, err := os.Stat(p); err != nil {
			return nil, fmt.Errorf("no %s share in %s", role, dir)
		}
		paths = append(paths, p)
	}
	return readShares(paths)
}

func anyShare(shares map[string]mpc.Share) mpc.Share {
	for _, role := range mpc.Roles {
		if s, ok := shares[role]; ok {
			return s
		}
	}
	return mpc.Share{}
}

// ---------------------------------------------------------------- the chain

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
		Data    string `json:"data"`
	} `json:"error"`
}

func rpcCall(node, method string, params map[string]any) (json.RawMessage, error) {
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": method, "params": params,
	})
	if err != nil {
		return nil, err
	}
	resp, err := http.Post(strings.TrimRight(node, "/"), "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out rpcResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s: %s", method, strings.TrimSpace(string(raw)))
	}
	if out.Error != nil {
		return nil, fmt.Errorf("%s: %s %s", method, out.Error.Message, out.Error.Data)
	}
	return out.Result, nil
}

// accountOf reads the account number and sequence over ABCI.
//
// ABCI rather than REST on purpose: this deployment's REST surface is
// allowlisted per module path behind the proxy and returns 401 for auth, so a
// tool that reached for it would work on a laptop and fail on the thing it is
// meant to operate.
func accountOf(node string, cdc codec.Codec, address string) (uint64, uint64, error) {
	req, err := cdc.Marshal(&authtypes.QueryAccountRequest{Address: address})
	if err != nil {
		return 0, 0, err
	}
	result, err := rpcCall(node, "abci_query", map[string]any{
		"path": "/cosmos.auth.v1beta1.Query/Account",
		"data": hex.EncodeToString(req),
	})
	if err != nil {
		return 0, 0, err
	}
	var wrapper struct {
		Response struct {
			Code  uint32 `json:"code"`
			Log   string `json:"log"`
			Value string `json:"value"`
		} `json:"response"`
	}
	if err := json.Unmarshal(result, &wrapper); err != nil {
		return 0, 0, err
	}
	if wrapper.Response.Code != 0 {
		return 0, 0, fmt.Errorf(
			"%s (an account the chain has never seen has no number; send it something first)",
			strings.TrimSpace(wrapper.Response.Log))
	}
	value, err := base64.StdEncoding.DecodeString(wrapper.Response.Value)
	if err != nil {
		return 0, 0, err
	}
	var res authtypes.QueryAccountResponse
	if err := cdc.Unmarshal(value, &res); err != nil {
		return 0, 0, err
	}
	var acc sdk.AccountI
	if err := cdc.UnpackAny(res.Account, &acc); err != nil {
		return 0, 0, err
	}
	return acc.GetAccountNumber(), acc.GetSequence(), nil
}

func broadcast(node string, txBytes []byte) (string, uint32, string, error) {
	result, err := rpcCall(node, "broadcast_tx_sync", map[string]any{
		"tx": base64.StdEncoding.EncodeToString(txBytes),
	})
	if err != nil {
		return "", 0, "", err
	}
	var out struct {
		Code uint32 `json:"code"`
		Hash string `json:"hash"`
		Log  string `json:"log"`
	}
	if err := json.Unmarshal(result, &out); err != nil {
		return "", 0, "", err
	}
	return out.Hash, out.Code, out.Log, nil
}
