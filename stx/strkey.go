// The stx package provides a compiled go representation of Stellar's
// XDR data structures.	 These data structures were generated by goxdr
// (https://github.com/xdrpp/goxdr/), which is documented at
// https://xdrpp.github.io/goxdr/pkg/github.com/xdrpp/goxdr/cmd/goxdr/goxdr.1.html.
// If you wish to bypass the stc library to mashal these structures
// directly, you will want to import "github.com/xdrpp/goxdr/xdr".
package stx

import (
	"bytes"
	"encoding/base32"
	"fmt"
	"github.com/xdrpp/goxdr/xdr"
	"io"
	"strings"
)

type StrKeyError string

func (e StrKeyError) Error() string { return string(e) }

type StrKeyVersionByte byte

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

const (
	STRKEY_ALG_ED25519 = 0
)

const (
	STRKEY_PUBKEY	   StrKeyVersionByte = 6 << 3  // 'G'
	STRKEY_MUXED	   StrKeyVersionByte = 12 << 3 // 'M'
	STRKEY_PRIVKEY	   StrKeyVersionByte = 18 << 3 // 'S'
	STRKEY_PRE_AUTH_TX StrKeyVersionByte = 19 << 3 // 'T',
	STRKEY_HASH_X	   StrKeyVersionByte = 23 << 3 // 'X'
	STRKEY_SIGNED_PAYLOAD StrKeyVersionByte = 15 << 3 // 'P'
	STRKEY_ERROR	   StrKeyVersionByte = 255
)

var payloadLen = map[StrKeyVersionByte]int{
	STRKEY_PUBKEY | STRKEY_ALG_ED25519:	 32,
	STRKEY_MUXED | STRKEY_ALG_ED25519:	 40,
	STRKEY_PRIVKEY | STRKEY_ALG_ED25519: 32,
	STRKEY_PRE_AUTH_TX:					 32,
	STRKEY_HASH_X:						 32,
	STRKEY_SIGNED_PAYLOAD:				 -1,
}

var crc16table [256]uint16

func init() {
	const poly = 0x1021
	for i := 0; i < 256; i++ {
		crc := uint16(i) << 8
		for j := 0; j < 8; j++ {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ poly
			} else {
				crc <<= 1
			}
		}
		crc16table[i] = crc
	}
}

func crc16(data []byte) (crc uint16) {
	for _, b := range data {
		temp := b ^ byte(crc>>8)
		crc = crc16table[temp] ^ (crc << 8)
	}
	return
}

// ToStrKey converts the raw bytes of a key to ASCII strkey format.
func ToStrKey(ver StrKeyVersionByte, bin []byte) string {
	var out bytes.Buffer
	out.WriteByte(byte(ver))
	out.Write(bin)
	sum := crc16(out.Bytes())
	out.WriteByte(byte(sum))
	out.WriteByte(byte(sum >> 8))
	return b32.EncodeToString(out.Bytes())
}

// FromStrKey decodes a strkey-format string into the raw bytes of the
// key and the type of key.	 Returns the reserved StrKeyVersionByte
// STRKEY_ERROR if it fails to decode the string.
func FromStrKey(in []byte) ([]byte, StrKeyVersionByte) {
	if rem := len(in) % 8; rem == 1 || rem == 3 || rem == 6 {
		return nil, STRKEY_ERROR
	}
	bin := make([]byte, b32.DecodedLen(len(in)))
	n, err := b32.Decode(bin, in)
	if err != nil || n != len(bin) || n < 3 {
		return nil, STRKEY_ERROR
	}
	if targetlen, ok := payloadLen[StrKeyVersionByte(bin[0])]; !ok ||
		(targetlen != -1 && targetlen != n-3) {
		return nil, STRKEY_ERROR
	}
	want := uint16(bin[len(bin)-2]) | uint16(bin[len(bin)-1])<<8
	if want != crc16(bin[:len(bin)-2]) {
		return nil, STRKEY_ERROR
	}
	if len(bin)%5 != 0 {
		// XXX - only really need to re-encode the last n - (n%5) bytes
		check := make([]byte, len(in))
		b32.Encode(check, bin)
		if in[len(in)-1] != check[len(check)-1] {
			return nil, STRKEY_ERROR
		}
	}
	return bin[1 : len(bin)-2], StrKeyVersionByte(bin[0])
}

func XdrToBytes(ts ...xdr.XdrType) []byte {
	out := bytes.Buffer{}
	for _, t := range ts {
		t.XdrMarshal(&xdr.XdrOut{&out}, "")
	}
	return out.Bytes()
}

func XdrFromBytes(input []byte, ts ...xdr.XdrType) (err error) {
	defer func() {
		if i := recover(); i != nil {
			if xe, ok := i.(error); ok {
				err = xe
				return
			}
			panic(i)
		}
	}()
	in := bytes.NewReader(input)
	for _, t := range ts {
		t.XdrMarshal(&xdr.XdrIn{in}, "")
	}
	return
}

// Renders a PublicKey in strkey format.
func (pk PublicKey) String() string {
	switch pk.Type {
	case PUBLIC_KEY_TYPE_ED25519:
		return ToStrKey(STRKEY_PUBKEY|STRKEY_ALG_ED25519, pk.Ed25519()[:])
	default:
		return fmt.Sprintf("PublicKey.Type#%d", int32(pk.Type))
	}
}

// Renders a MuxedAccount in strkey format.
func (pk MuxedAccount) String() string {
	switch pk.Type {
	case KEY_TYPE_ED25519:
		return ToStrKey(STRKEY_PUBKEY|STRKEY_ALG_ED25519, pk.Ed25519()[:])
	case KEY_TYPE_MUXED_ED25519:
		return ToStrKey(STRKEY_MUXED|STRKEY_ALG_ED25519,
			XdrToBytes(XDR_Uint256(&pk.Med25519().Ed25519),
				XDR_Uint64(&pk.Med25519().Id)))
	default:
		return fmt.Sprintf("MuxedAccount.Type#%d", int32(pk.Type))
	}
}

// Renders a SignerKey in strkey format.
func (pk SignerKey) String() string {
	switch pk.Type {
	case SIGNER_KEY_TYPE_ED25519:
		return ToStrKey(STRKEY_PUBKEY|STRKEY_ALG_ED25519, pk.Ed25519()[:])
	case SIGNER_KEY_TYPE_PRE_AUTH_TX:
		return ToStrKey(STRKEY_PRE_AUTH_TX, pk.PreAuthTx()[:])
	case SIGNER_KEY_TYPE_HASH_X:
		return ToStrKey(STRKEY_HASH_X, pk.HashX()[:])
	case SIGNER_KEY_TYPE_ED25519_SIGNED_PAYLOAD:
		// Strip 4 types (SignerKey discriminant) from XDR encoding
		return ToStrKey(STRKEY_SIGNED_PAYLOAD|STRKEY_ALG_ED25519,
			XdrToBytes(&pk)[4:])
	default:
		return fmt.Sprintf("SignerKey.Type#%d", int32(pk.Type))
	}
}

func renderByte(b byte) string {
	if b <= ' ' || b >= '\x7f' {
		return fmt.Sprintf("\\x%02x", b)
	} else if b == '\\' || b == ':' {
		return "\\" + string(b)
	}
	return string(b)
}

func RenderAssetCode(bs []byte) string {
	var n int
	for n = len(bs); n > 0 && bs[n-1] == 0; n-- {
	}
	if len(bs) > 4 && n <= 4 {
		n = 5
	}
	out := &strings.Builder{}
	for i := 0; i < n; i++ {
		out.WriteString(renderByte(bs[i]))
	}
	return out.String()
}

// Renders an Asset as Code:AccountID.
func (a Asset) String() string {
	var code []byte
	var issuer *AccountID
	switch a.Type {
	case ASSET_TYPE_NATIVE:
		return "native"
	case ASSET_TYPE_CREDIT_ALPHANUM4:
		code = a.AlphaNum4().AssetCode[:]
		issuer = &a.AlphaNum4().Issuer
	case ASSET_TYPE_CREDIT_ALPHANUM12:
		code = a.AlphaNum12().AssetCode[:]
		issuer = &a.AlphaNum12().Issuer
	default:
		return fmt.Sprintf("Asset.Type#%d", int32(a.Type))
	}
	return fmt.Sprintf("%s:%s", RenderAssetCode(code), issuer.String())
}

// Renders a TrustLineAsset as Code:AccountID or hexid:lp.
func (a TrustLineAsset) String() string {
	var code []byte
	var issuer *AccountID
	switch a.Type {
	case ASSET_TYPE_NATIVE:
		return "native"
	case ASSET_TYPE_CREDIT_ALPHANUM4:
		code = a.AlphaNum4().AssetCode[:]
		issuer = &a.AlphaNum4().Issuer
	case ASSET_TYPE_CREDIT_ALPHANUM12:
		code = a.AlphaNum12().AssetCode[:]
		issuer = &a.AlphaNum12().Issuer
	case ASSET_TYPE_POOL_SHARE:
		return fmt.Sprintf("%x:lp", a.LiquidityPoolID()[:])
	default:
		return fmt.Sprintf("Asset.Type#%d", int32(a.Type))
	}
	return fmt.Sprintf("%s:%s", RenderAssetCode(code), issuer.String())
}

func (a AssetCode) String() string {
	switch a.Type {
	case ASSET_TYPE_CREDIT_ALPHANUM4:
		return RenderAssetCode(a.AssetCode4()[:])
	case ASSET_TYPE_CREDIT_ALPHANUM12:
		return RenderAssetCode(a.AssetCode12()[:])
	default:
		return fmt.Sprintf("AllowTrustOp_Asset.Type#%d", int32(a.Type))
	}
}

func ScanAssetCode(input []byte) ([]byte, error) {
	out := make([]byte, 12)
	ss := bytes.NewReader(input)
	var i int
	r := byte(' ')
	var err error
	for i = 0; i < len(out); i++ {
		r, err = ss.ReadByte()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		} else if r <= 32 || r >= 127 {
			return nil, StrKeyError("Invalid character in AssetCode")
		} else if r != '\\' {
			out[i] = byte(r)
			continue
		}
		r, err = ss.ReadByte()
		if err != nil {
			return nil, err
		} else if r != 'x' {
			out[i] = byte(r)
		} else if _, err = fmt.Fscanf(ss, "%02x", &out[i]); err != nil {
			return nil, err
		}
	}
	if ss.Len() > 0 {
		return nil, StrKeyError("AssetCode too long")
	}
	if i <= 4 {
		return out[:4], nil
	}
	return out, nil
}

type unexpectedLiquidityPoolId struct {
	id PoolID
}
func (e unexpectedLiquidityPoolId) Error() string {
	return fmt.Sprintf("unexpected LiquidityPoolID %x for Asset", e.id[:])
}

func (a *Asset) Scan(ss fmt.ScanState, _ rune) error {
	bs, err := ss.Token(true, nil)
	if err != nil {
		return err
	}
	colon := bytes.LastIndexByte(bs, ':')
	if colon == -1 {
		if len(bs) > 12 {
			return StrKeyError("Asset should be Code:AccountID or native")
		}
		a.Type = ASSET_TYPE_NATIVE
		return nil
	}
	if string(bs[colon+1:]) == "lp" {
		var ulpid unexpectedLiquidityPoolId
		if _, err = fmt.Sscan(string(bs[:colon]),
			XDR_PoolID(&ulpid.id)); err == nil {
			return ulpid
		}
	}
	var issuer AccountID
	if _, err = fmt.Fscan(bytes.NewReader(bs[colon+1:]), &issuer); err != nil {
		return err
	}
	code, err := ScanAssetCode(bs[:colon])
	if err != nil {
		return err
	}
	if len(code) <= 4 {
		a.Type = ASSET_TYPE_CREDIT_ALPHANUM4
		copy(a.AlphaNum4().AssetCode[:], code)
		a.AlphaNum4().Issuer = issuer
	} else {
		a.Type = ASSET_TYPE_CREDIT_ALPHANUM12
		copy(a.AlphaNum12().AssetCode[:], code)
		a.AlphaNum12().Issuer = issuer
	}
	return nil
}

func (a *TrustLineAsset) Scan(ss fmt.ScanState, r rune) error {
	var aa Asset
	switch v := aa.Scan(ss, r).(type) {
	case unexpectedLiquidityPoolId:
		a.Type = ASSET_TYPE_POOL_SHARE
		*a.LiquidityPoolID() = v.id
	case nil:
		a.Type = aa.Type
		a._u = aa._u
	default:
		return v
	}
	return nil
}

func (a *AssetCode) Scan(ss fmt.ScanState, _ rune) error {
	bs, err := ss.Token(true, nil)
	code, err := ScanAssetCode(bs)
	if err != nil {
		return err
	}
	if len(code) <= 4 {
		a.Type = ASSET_TYPE_CREDIT_ALPHANUM4
		copy(a.AssetCode4()[:], code)
	} else {
		a.Type = ASSET_TYPE_CREDIT_ALPHANUM12
		copy(a.AssetCode12()[:], code)
	}
	return nil
}

// Returns true if c is a valid character in a strkey formatted key.
func IsStrKeyChar(c rune) bool {
	return c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// Parses a public key in strkey format.
func (pk *PublicKey) Scan(ss fmt.ScanState, _ rune) error {
	bs, err := ss.Token(true, IsStrKeyChar)
	if err != nil {
		return err
	}
	return pk.UnmarshalText(bs)
}

// Parses a muxedaccount in strkey format.
func (pk *MuxedAccount) Scan(ss fmt.ScanState, _ rune) error {
	bs, err := ss.Token(true, IsStrKeyChar)
	if err != nil {
		return err
	}
	return pk.UnmarshalText(bs)
}

// Parses a signer in strkey format.
func (pk *SignerKey) Scan(ss fmt.ScanState, _ rune) error {
	bs, err := ss.Token(true, IsStrKeyChar)
	if err != nil {
		return err
	}
	return pk.UnmarshalText(bs)
}

// Parses a public key in strkey format.
func (pk *PublicKey) UnmarshalText(bs []byte) error {
	key, vers := FromStrKey(bs)
	switch vers {
	case STRKEY_PUBKEY | STRKEY_ALG_ED25519:
		pk.Type = PUBLIC_KEY_TYPE_ED25519
		copy(pk.Ed25519()[:], key)
		return nil
	default:
		return StrKeyError("Invalid public key type")
	}
}

// Parses a MuxedAccount in strkey format.
func (pk *MuxedAccount) UnmarshalText(bs []byte) error {
	key, vers := FromStrKey(bs)
	switch vers {
	case STRKEY_PUBKEY | STRKEY_ALG_ED25519:
		pk.Type = KEY_TYPE_ED25519
		copy(pk.Ed25519()[:], key)
		return nil
	case STRKEY_MUXED | STRKEY_ALG_ED25519:
		pk.Type = KEY_TYPE_MUXED_ED25519
		if err := XdrFromBytes(key,
			XDR_Uint256(&pk.Med25519().Ed25519),
			XDR_Uint64(&pk.Med25519().Id)); err != nil {
			return err
		}
		return nil
	default:
		return StrKeyError("Invalid public key type")
	}
}

// Parses a signer in strkey format.
func (pk *SignerKey) UnmarshalText(bs []byte) (err error) {
	key, vers := FromStrKey(bs)
	switch vers {
	case STRKEY_PUBKEY | STRKEY_ALG_ED25519:
		pk.Type = SIGNER_KEY_TYPE_ED25519
		copy(pk.Ed25519()[:], key)
	case STRKEY_PRE_AUTH_TX:
		pk.Type = SIGNER_KEY_TYPE_PRE_AUTH_TX
		copy(pk.PreAuthTx()[:], key)
	case STRKEY_HASH_X:
		pk.Type = SIGNER_KEY_TYPE_HASH_X
		copy(pk.HashX()[:], key)
	case STRKEY_SIGNED_PAYLOAD:
		defer func() {
			if i := recover(); i != nil {
				if _, ok := i.(xdr.XdrError); ok {
					err = StrKeyError("Invalid signer key string")
				}
				panic(i)
			}
		}()
		pk.Type = SIGNER_KEY_TYPE_ED25519_SIGNED_PAYLOAD
		pk.Ed25519SignedPayload().
			XdrMarshal(&xdr.XdrIn{bytes.NewReader(key)}, "")
	default:
		err = StrKeyError("Invalid signer key string")
	}
	return
}

func signerHint(bs []byte) (ret SignatureHint) {
	if len(bs) < 4 {
		panic(StrKeyError("signerHint insufficient signer length"))
	}
	copy(ret[:], bs[len(bs)-4:])
	return
}

// Returns the last 4 bytes of a PublicKey, as required for the Hint
// field in a DecoratedSignature.
func (pk PublicKey) Hint() SignatureHint {
	switch pk.Type {
	case PUBLIC_KEY_TYPE_ED25519:
		return signerHint(pk.Ed25519()[:])
	default:
		panic(StrKeyError("Invalid public key type"))
	}
}

// Returns the last 4 bytes of a SignerKey, as required for the Hint
// field in a DecoratedSignature.
func (pk SignerKey) Hint() SignatureHint {
	switch pk.Type {
	case SIGNER_KEY_TYPE_ED25519:
		return signerHint(pk.Ed25519()[:])
	case SIGNER_KEY_TYPE_PRE_AUTH_TX:
		return signerHint(pk.PreAuthTx()[:])
	case SIGNER_KEY_TYPE_HASH_X:
		return signerHint(pk.HashX()[:])
	case SIGNER_KEY_TYPE_ED25519_SIGNED_PAYLOAD:
		hint := signerHint(pk.Ed25519SignedPayload().Ed25519[:])
		pl := pk.Ed25519SignedPayload().Payload
		if len(pl) > 4 {
			pl = pl[len(pl)-4:]
		}
		for i, b := range pl {
			hint[i] ^= b
		}
		return hint
	default:
		panic(StrKeyError("Invalid signer key type"))
	}
}
