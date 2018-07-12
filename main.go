package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"golang.org/x/crypto/ssh/terminal"
	"io/ioutil"
	"os"
	"strings"
)

func txOut(e *TransactionEnvelope) string {
	out := &strings.Builder{}
	b64o := base64.NewEncoder(base64.StdEncoding, out)
	e.XdrMarshal(&XdrOut{b64o}, "")
	b64o.Close()
	return out.String()
}

func txIn(input string) *TransactionEnvelope {
	in := strings.NewReader(input)
	var e TransactionEnvelope
	b64i := base64.NewDecoder(base64.StdEncoding, in)
	e.XdrMarshal(&XdrIn{b64i}, "")
	return &e
}

func txString(t XdrAggregate) string {
	out := &strings.Builder{}
	t.XdrMarshal(&XdrPrint{out}, "")
	return out.String()
}

type XdrScan struct {
	kvs map[string]string
}

func (*XdrScan) Sprintf(f string, args ...interface{}) string {
	return fmt.Sprintf(f, args...)
}

type xdrPointer interface{
	XdrPointer() interface{}
}

func (xs *XdrScan) Marshal(name string, i interface{}) {
	val, ok := xs.kvs[name]
	switch v := i.(type) {
	case fmt.Scanner:
		if !ok { return }
		_, err := fmt.Sscan(val, v)
		if err != nil {
			xdrPanic("%s", err.Error())
		}
	case XdrPtr:
		val = xs.kvs[name + ".present"]
		for len(val) > 0 && val[0] == ' ' {
			val = val[1:]
		}
		switch val {
		case "false", "":
			v.SetPresent(false)
		case "true":
			v.SetPresent(true)
		default:
			xdrPanic("%s.present (%s) must be true or false", name,
				xs.kvs[name + ".present"])
		}
		v.XdrMarshalValue(xs, name)

	case *XdrSize:
		fmt.Sscan(xs.kvs[name + ".len"], v.XdrPointer())
	case XdrAggregate:
		v.XdrMarshal(xs, name)
	case xdrPointer:
		if !ok { return }
		fmt.Sscan(val, v.XdrPointer())
	default:
		xdrPanic("XdrScan: Don't know how to parse %s\n", name)
	}
	delete(xs.kvs, name)
}

func txScan(t XdrAggregate, in string) {
	kvs := map[string]string{}
	lineno := 0
	for _, line := range strings.Split(in, "\n") {
		lineno++
		kv := strings.SplitN(line, ":", 2)
		if len(kv) != 2 {
			continue
		}
		kvs[kv[0]] = kv[1]
	}
	t.XdrMarshal(&XdrScan{kvs}, "")
}

func doKeyGen() {
	sk := KeyGen(PUBLIC_KEY_TYPE_ED25519)
	fmt.Println(sk)
	fmt.Println(sk.Public())
	fmt.Printf("%x\n", sk.Public().Hint())
}

var progname string

func fixTx(XXX interface{}, e *TransactionEnvelope) {
	feechan := make(chan uint32)
	go func() {
		if h := GetLedgerHeader(); h != nil {
			feechan <- h.BaseFee
		} else {
			feechan <- 0
		}
	}()

	seqchan := make(chan SequenceNumber)
	go func() {
		var zero AccountID
		if e.Tx.SourceAccount == zero {
			seqchan <- 0
		} else if a := GetAccountEntry(e.Tx.SourceAccount.String()); a != nil {
			var val SequenceNumber
			fmt.Sscan(a.Sequence.String(), &val)
			seqchan <- val
		} else {
			seqchan <- 0
		}
	}()

	if newfee := uint32(len(e.Tx.Operations)) * <-feechan; newfee > e.Tx.Fee {
		e.Tx.Fee = newfee
	}
	if newseq := <-seqchan; newseq > e.Tx.SeqNum {
		e.Tx.SeqNum = newseq
	}
}

func main() {
	opt_compile := flag.Bool("c", false, "Compile output to binary XDR")
	opt_decompile := flag.Bool("d", false, "Decompile input from binary XDR")
	opt_keygen := flag.Bool("keygen", false, "Create a new signing keypair")
	opt_output := flag.String("o", "", "Output to file instead of stdout")
	opt_preauth := flag.Bool("preauth", false,
		"Hash transaction for pre-auth use")
	opt_inplace := flag.Bool("i", false,
		"Edit the input file (required) in place")
	opt_sign := flag.Bool("sign", false, "Sign the transaction")
	opt_testnet := flag.Bool("testnet", false, "Sign/hash for testnet")
	opt_net := flag.Bool("n", false,
		"Query network to update 0 fee and sequence number")
	if pos := strings.LastIndexByte(os.Args[0], '/'); pos >= 0 {
		progname = os.Args[0][pos+1:]
	} else {
		progname = os.Args[0]
	}
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(),
			"Usage: %[1]s [-sign] [-c] [-d] [-i | -o FILE] [INPUT-FILE]\n" +
			"       %[1]s [-preauth] [-d] [INPUT-FILE]\n" +
			"       %[1]s [-keygen]\n", progname)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *opt_preauth && *opt_output != "" ||
		*opt_keygen && (*opt_compile || *opt_decompile || *opt_preauth ||
		*opt_inplace) {
		flag.Usage()
	}
	if *opt_inplace {
		if *opt_output != "" || len(flag.Args()) == 0 {
			flag.Usage()
		}
		*opt_output = flag.Args()[0]
	}

	if (*opt_keygen) {
		doKeyGen()
		return
	}

	var input []byte
	var err error
	switch (len(flag.Args())) {
	case 0:
		input, err = ioutil.ReadAll(os.Stdin)
	case 1:
		input, err = ioutil.ReadFile(flag.Args()[0])
	default:
		flag.Usage()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}
	sinput := string(input)

	if !*opt_decompile && len(sinput) != 0 &&
		strings.IndexByte(sinput, ':') == -1 {
		if bs, err := base64.StdEncoding.DecodeString(sinput);
		err == nil && len(bs) > 0 {
			*opt_decompile = true
		}
	}

	var e *TransactionEnvelope
	if *opt_decompile {
		e = txIn(sinput)
	} else {
		e = &TransactionEnvelope{}
		txScan(e, sinput)
	}

	net := MainNet
	if *opt_testnet {
		net = TestNet
	}

	if *opt_net {
		fixTx(net, e)
	}

	if (*opt_preauth) {
		sk := SignerKey{ Type: SIGNER_KEY_TYPE_PRE_AUTH_TX }
		copy(sk.PreAuthTx()[:], TxPayloadHash(net, e))
		fmt.Printf("%x\n", *sk.PreAuthTx())
		fmt.Println(&sk)
		return
	}

	if *opt_sign {
		fmt.Print("Enter Password: ")
		bytePassword, err := terminal.ReadPassword(0)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		var sk PrivateKey
		if n, err := fmt.Sscan(string(bytePassword), &sk); n != 1 {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(sk.Public())
		if err = sk.SignTx(net, e); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	var output string
	if *opt_compile {
		output = txOut(e) + "\n"
	} else {
		output = txString(e)
	}

	if *opt_output == "" {
		fmt.Print(output)
	} else {
		os.Remove(*opt_output + "~")
		os.Link(*opt_output, *opt_output + "~")
		tmp := fmt.Sprintf("%s#%d#", *opt_output, os.Getpid())
		ioutil.WriteFile(tmp, []byte(output), 0666)
		os.Rename(tmp, *opt_output)
	}
}
