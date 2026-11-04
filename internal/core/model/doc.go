// Package model is the format-independent data model of a KeePass database
// (SPEC §6). The KDBX readers fill it and the writers read it; diff and merge work
// on it and know nothing of any file version.
//
// Every string value of an entry is held as a secret.Bytes, so that a value can
// be neither printed nor logged by accident; Field.Protected is the flag the file
// stored, which says whether clients mask the value. What the model has no field
// for is kept in Raw (SPEC §4.4), so that nothing a file holds is lost.
package model
