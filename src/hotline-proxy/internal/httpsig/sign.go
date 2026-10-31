// Package httpsig makes RFC 9421 HTTP message signatures with HMAC-SHA256.
package httpsig

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
	"strings"
)

// Component is one line of the signature base: a component's name, an
// optional parameter such as resp, and its value.
type Component struct {
	Name  string
	Param string
	Value string
}

// Params are the signature's own parameters. Alg is left out when empty.
type Params struct {
	Created int64
	KeyID   string
	Alg     string
}

// HMACSHA256 signs the components in their order and returns the
// signature in base64 and the signature input, as RFC 9421 writes them.
func HMACSHA256(key []byte, components []Component, params Params) (signature, input string) {
	var base strings.Builder
	names := make([]string, len(components))
	for i, c := range components {
		names[i] = strconv.Quote(c.Name)
		if c.Param != "" {
			names[i] += ";" + c.Param
		}
		base.WriteString(names[i] + ": " + c.Value + "\n")
	}
	input = "(" + strings.Join(names, " ") + ");created=" + strconv.FormatInt(params.Created, 10) +
		";keyid=" + strconv.Quote(params.KeyID)
	if params.Alg != "" {
		input += ";alg=" + strconv.Quote(params.Alg)
	}
	base.WriteString(`"@signature-params": ` + input)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(base.String()))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), input
}
