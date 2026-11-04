package crypto

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"io"
	"strings"

	"github.com/rm4n0s/errors"
)

// keyFileKey returns the key a key file stands for, as KeePass reads it. In this
// order:
//
//   - an XML key file (a well-formed document whose root is KeyFile), version 1.x
//     with the key in base64 or version 2.x with the key as 64 hex digits and a
//     Hash attribute that is checked: the key it holds;
//   - any other file of 32 bytes: the 32 bytes;
//   - any other file of 64 bytes that are all hexadecimal digits: the 32 bytes they
//     spell;
//   - any other file: the SHA-256 of its content.
//
// A document that is XML with a KeyFile root but is damaged inside (a missing
// element, bad base64 or hex, a Hash that does not match) is KeyFileCorrupt; it
// is not silently read as a binary file, because that would give a key that no
// other program gives.
func keyFileKey(data []byte) ([]byte, error) {
	if key, isXML, err := xmlKeyFileKey(data); isXML {
		return key, err
	}
	if len(data) == 32 {
		return append([]byte(nil), data...), nil
	}
	if len(data) == 64 {
		if key, err := hex.DecodeString(string(data)); err == nil {
			return key, nil
		}
	}
	sum := sha256.Sum256(data)
	return sum[:], nil
}

// xmlKey is the document of an XML key file.
type xmlKey struct {
	XMLName xml.Name `xml:"KeyFile"`
	Meta    struct {
		Version string `xml:"Version"`
	} `xml:"Meta"`
	Key struct {
		Data struct {
			Hash  string `xml:"Hash,attr"`
			Value string `xml:",chardata"`
		} `xml:"Data"`
	} `xml:"Key"`
}

// xmlKeyFileKey reads data as an XML key file. isXML is false when data is not
// one (not well-formed XML, or another root element), and then the other rules
// apply; when it is true, the key or the KeyFileCorrupt error is the answer.
func xmlKeyFileKey(data []byte) (key []byte, isXML bool, err error) {
	// A key file is small; a quick look keeps binary files out of the XML parser.
	if !bytes.Contains(data, []byte("KeyFile")) {
		return nil, false, nil
	}
	var doc xmlKey
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	if derr := dec.Decode(&doc); derr != nil {
		// Either not XML at all, or XML with another root: binary rules apply. An
		// XML key file with the right root that fails half way is reported below
		// only when the root was seen, which Decode tells by filling XMLName.
		if doc.XMLName.Local != "KeyFile" || derr == io.EOF {
			return nil, false, nil
		}
		return nil, true, errors.NewErr("KeyFileCorrupt", derr)
	}
	version := strings.TrimSpace(doc.Meta.Version)
	value := strings.Join(strings.Fields(doc.Key.Data.Value), "")
	switch {
	case strings.HasPrefix(version, "1."):
		raw, derr := base64.StdEncoding.DecodeString(value)
		if derr != nil || len(raw) == 0 {
			return nil, true, errors.New("KeyFileCorrupt", "the key of the XML key file is not valid base64", "version", version)
		}
		return raw, true, nil
	case strings.HasPrefix(version, "2."):
		raw, derr := hex.DecodeString(value)
		if derr != nil || len(raw) != 32 {
			return nil, true, errors.New("KeyFileCorrupt", "the key of the XML key file is not 64 hexadecimal digits", "version", version)
		}
		want, derr := hex.DecodeString(strings.TrimSpace(doc.Key.Data.Hash))
		sum := sha256.Sum256(raw)
		if derr != nil || len(want) != 4 || !bytes.Equal(want, sum[:4]) {
			clear(raw)
			return nil, true, errors.New("KeyFileCorrupt", "the checksum of the XML key file does not match its key: the file is damaged", "version", version)
		}
		return raw, true, nil
	}
	return nil, true, errors.New("KeyFileCorrupt", "the XML key file has an unknown version", "version", version)
}
