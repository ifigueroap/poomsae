package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"strings"
)

const nativeAgentHookDigestPrefix = "// poomsae-generated-sha256: "

// legacyAgentHookDigestPrefix is the digest marker written by kata. Existing
// generated adapters keep that marker until poomsae rewrites them.
const legacyAgentHookDigestPrefix = "// kata-generated-sha256: "

// recognizedMetadataPrefix returns the ownership prefix on line. Kata installs
// keep the older prefix until poomsae rewrites the adapter.
func recognizedMetadataPrefix(line []byte, current, legacy string) ([]byte, bool) {
	for _, prefix := range []string{current, legacy} {
		if bytes.HasPrefix(line, []byte(prefix)) {
			return []byte(prefix), true
		}
	}
	return nil, false
}

// recognizedGeneratedFormat reports whether format is the current name or the
// kata name of the same adapter.
func recognizedGeneratedFormat(got, current string) bool {
	if got == current {
		return true
	}
	return got == strings.Replace(current, "poomsae-", "kata-", 1)
}

// Metadata embedded in JavaScript comments and literals must escape line
// separators. Deterministic encoding also keeps ownership records stable.
var nativeAgentHookOwnedJSONOptions = json.JoinOptions(
	jsontext.EscapeForHTML(true), jsontext.EscapeForJS(true), jsontext.AllowInvalidUTF8(true),
	json.FormatNilSliceAsNull(true), json.FormatNilMapAsNull(true), json.Deterministic(true),
)

// Hash the stored metadata and runtime, rather than reconstructing today's
// template. Lengths separate bundle members; the digest itself is excluded.
// This detects local edits, not authenticity against a repository writer.
func nativeAgentHookDigest(files ...[]byte) string {
	hash := sha256.New()
	for _, data := range files {
		_, _ = fmt.Fprintf(hash, "%d:", len(data))
		_, _ = hash.Write(data)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func sealNativeAgentHookCode(code []byte, companions ...[]byte) []byte {
	digest := nativeAgentHookDigest(append([][]byte{code}, companions...)...)
	return append(code, []byte(nativeAgentHookDigestPrefix+digest+"\n")...)
}

func nativeAgentHookCodeUnedited(code []byte, companions ...[]byte) bool {
	marker, prefix := -1, ""
	for _, candidate := range []string{nativeAgentHookDigestPrefix, legacyAgentHookDigestPrefix} {
		index := bytes.LastIndex(code, []byte("\n"+candidate))
		if index > marker {
			marker, prefix = index, candidate
		}
	}
	if marker < 0 {
		return false
	}
	body := code[:marker+1]
	digest := nativeAgentHookDigest(append([][]byte{body}, companions...)...)
	return string(code[marker+1:]) == prefix+digest+"\n"
}
