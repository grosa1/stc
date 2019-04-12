/*

Stellar transaction compiler library.  Provides functions for
manipulating Stellar transactions, translating them back and forth
between txrep format, and posting them.

*/
package stc

import (
	"fmt"
	"github.com/xdrpp/stc/stcdetail"
	"github.com/xdrpp/stc/stx"
	"strings"
)

type TxrepError = stcdetail.TxrepError
type PublicKey = stx.PublicKey
type SignerKey = stx.SignerKey
type Signature = stx.Signature
type TransactionResult = stx.TransactionResult
type LedgerHeader = stx.LedgerHeader

type TransactionEnvelope struct {
	*stx.TransactionEnvelope
	Help map[string]struct{}
}

func NewTransactionEnvelope() *TransactionEnvelope {
	return &TransactionEnvelope{
		TransactionEnvelope: &stx.TransactionEnvelope{},
		Help:                nil,
	}
}

func (txe *TransactionEnvelope) GetHelp(name string) bool {
	_, ok := txe.Help[name]
	return ok
}

func (txe *TransactionEnvelope) SetHelp(name string) {
	if txe.Help == nil {
		txe.Help = map[string]struct{}{name: struct{}{}}
	} else {
		txe.Help[name] = struct{}{}
	}
}

type txrepHelper = StellarNet

func (net *txrepHelper) SignerNote(txe *stx.TransactionEnvelope,
	sig *stx.DecoratedSignature) string {
	if txe == nil {
		return ""
	} else if ski := net.Signers.Lookup(net.GetNetworkId(), txe, sig);
	ski != nil {
		return ski.String()
	}
	return fmt.Sprintf("bad signature/unknown key/%s is wrong network",
		net.Name)
}

func (net *txrepHelper) AccountIDNote(acct *stx.AccountID) string {
	return net.Accounts[acct.String()]
}

// Convert an arbitrary XDR data structure to human-readable Txrep
// format.
func (net *StellarNet) ToRep(txe stx.XdrAggregate) string {
	var out strings.Builder

	type helper interface {
		stx.XdrAggregate
		GetHelp(string) bool
	}
	if e, ok := txe.(helper); ok {
		ntxe := struct {
			helper
			*txrepHelper
		}{e, (*txrepHelper)(net)}
		stcdetail.XdrToTxrep(&out, ntxe)
	} else {
		ntxe := struct {
			stx.XdrAggregate
			*txrepHelper
		}{txe, (*txrepHelper)(net)}
		stcdetail.XdrToTxrep(&out, ntxe)
	}

	return out.String()
}

// Convert a TransactionEnvelope to human-readable Txrep format.
func (net *StellarNet) TxToRep(txe *TransactionEnvelope) string {
	return net.ToRep(txe)
}

// Parse a transaction in human-readable Txrep format into a
// TransactionEnvelope.
func TxFromRep(rep string) (*TransactionEnvelope, TxrepError) {
	in := strings.NewReader(rep)
	txe := NewTransactionEnvelope()
	if err := stcdetail.XdrFromTxrep(in, txe); err != nil {
		return txe, err
	}
	return txe, nil
}

// Convert a TransactionEnvelope to base64-encoded binary XDR format.
func TxToBase64(tx *TransactionEnvelope) string {
	return stcdetail.XdrToBase64(tx)
}

// Parse a TransactionEnvelope from base64-encoded binary XDR format.
func TxFromBase64(input string) (*TransactionEnvelope, error) {
	tx := NewTransactionEnvelope()
	if err := stcdetail.XdrFromBase64(tx, input); err != nil {
		return nil, err
	}
	return tx, nil
}

type forEachXdr struct{
	fn func(stx.XdrType)bool
}
func (fex forEachXdr) Marshal(_ string, val stx.XdrType) {
	if !fex.fn(val) {
		if xa, ok := val.(stx.XdrAggregate); ok {
			xa.XdrMarshal(fex, "")
		}
	}
}
func (forEachXdr) Sprintf(string, ...interface{}) string {
	return ""
}

// Calls fn, recursively, on every value inside an XdrAggregate.
// Prunes the recursion if fn returns true.
func ForEachXdr(t stx.XdrAggregate, fn func(stx.XdrType)bool) {
	t.XdrMarshal(forEachXdr{fn}, "")
}
