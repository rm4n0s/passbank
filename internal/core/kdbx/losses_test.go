package kdbx

import (
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
)

// everythingDB holds one of each kind of data that some version lacks.
func everythingDB() *model.Database {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	yes := false
	entry := &model.Entry{
		UUID: uuid(10), Fields: []model.Field{{Key: model.FieldTitle, Value: secretOf("Bank")}},
		QualityCheck: &yes, PreviousParent: uuid(7), CustomData: []model.CustomDataItem{{Key: "k", Value: "v", LastModified: at}, {Key: "k2"}},
	}
	return &model.Database{
		Meta: model.Meta{
			SettingsChanged: at,
			CustomIcons:     []model.CustomIcon{{UUID: uuid(5), Name: "n", LastModified: at}},
			CustomData:      []model.CustomDataItem{{Key: "k", LastModified: at}},
		},
		PublicCustomData: []model.PublicCustomDataItem{{Key: "p", Type: 0x04, Data: []byte{1, 0, 0, 0}}},
		Root: &model.Group{
			UUID: uuid(1), Name: "Root",
			Groups: []*model.Group{{
				UUID: uuid(2), Name: "Sub", Tags: []string{"t"}, PreviousParent: uuid(1),
				CustomData: []model.CustomDataItem{{Key: "g", LastModified: at}},
				Entries:    []*model.Entry{entry},
			}},
		},
	}
}

func kinds(losses []Loss) string {
	var ks []string
	for _, l := range losses {
		ks = append(ks, l.Kind)
	}
	sort.Strings(ks)
	return strings.Join(ks, ",")
}

func TestLosses(t *testing.T) {
	all41 := []string{LossSettingsChanged, LossCustomIconName, LossCustomIconModified, LossCustomDataModified, LossGroupTags, LossGroupPreviousGroup, LossEntryPreviousGroup, LossEntryQualityCheck}
	tests := []struct {
		version Version
		want    []string // kinds, sorted below
	}{
		{Version{4, 1}, nil},
		// 4.0: the 4.1 fields; the custom data of the database (time), and the times of the items of the group and the entry.
		{Version{4, 0}, append(append([]string{}, all41...), LossCustomDataModified, LossCustomDataModified)},
		// 3.x and 2.x: the same, but the custom data of groups and entries goes as a whole, and the public custom data.
		{Version{3, 1}, append(append([]string{}, all41...), LossGroupCustomData, LossEntryCustomData, LossPublicCustomData)},
		{Version{3, 0}, append(append([]string{}, all41...), LossGroupCustomData, LossEntryCustomData, LossPublicCustomData)},
		{Version{2, 0}, append(append([]string{}, all41...), LossGroupCustomData, LossEntryCustomData, LossPublicCustomData)},
	}
	for _, tt := range tests {
		t.Run(tt.version.String(), func(t *testing.T) {
			got := Losses(everythingDB(), tt.version)
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if kinds(got) != strings.Join(want, ",") {
				t.Errorf("losses = %s\nwant      %s", kinds(got), strings.Join(want, ","))
			}
			for _, l := range got {
				if l.Item == "" || l.What == "" {
					t.Errorf("loss without an item or words: %+v", l)
				}
			}
		})
	}
}

func TestLossesNameTheItems(t *testing.T) {
	var tags, quality *Loss
	losses := Losses(everythingDB(), Version{4, 0})
	for i := range losses {
		switch losses[i].Kind {
		case LossGroupTags:
			tags = &losses[i]
		case LossEntryQualityCheck:
			quality = &losses[i]
		}
	}
	if tags == nil || tags.Item != "Root/Sub" || tags.UUID != uuid(2) || tags.What != "1 tag of a group" {
		t.Errorf("tags loss = %+v", tags)
	}
	if quality == nil || quality.Item != "Root/Sub/Bank" || quality.UUID != uuid(10) {
		t.Errorf("quality-check loss = %+v", quality)
	}
	// No value is in the text.
	for _, l := range losses {
		if strings.Contains(l.What, "\"v\"") {
			t.Errorf("a value is in %q", l.What)
		}
	}
}

func TestLossesInHistoryAndPlurals(t *testing.T) {
	yes := true
	db := &model.Database{Root: &model.Group{UUID: uuid(1), Name: "R", Entries: []*model.Entry{{
		UUID: uuid(2),
		History: []*model.Entry{
			{UUID: uuid(2), QualityCheck: &yes, CustomData: []model.CustomDataItem{{Key: "a"}, {Key: "b"}}},
			{UUID: uuid(2), PreviousParent: uuid(9)},
		},
	}}}}
	got := Losses(db, Version{3, 1})
	if kinds(got) != "EntryCustomData,EntryPreviousParent,EntryQualityCheck" {
		t.Fatalf("losses = %s", kinds(got))
	}
	for _, l := range got {
		if l.Kind == LossEntryCustomData && !strings.HasPrefix(l.What, "2 custom data items of an entry") {
			t.Errorf("What = %q", l.What)
		}
		if l.Kind == LossEntryQualityCheck && !strings.Contains(l.What, "history") {
			t.Errorf("What = %q", l.What)
		}
	}
	if got := kinds(Losses(db, Version{4, 0})); got != "EntryPreviousParent,EntryQualityCheck" {
		t.Errorf("4.0 losses = %s", got)
	}
	if len(Losses(&model.Database{}, Version{3, 1})) != 0 {
		t.Error("a database without a root group loses something")
	}
}

func TestLossesAgreeWithFit(t *testing.T) {
	versions := []Version{{4, 1}, {4, 0}, {3, 1}, {3, 0}, {2, 0}}
	o := modeltest.DefaultOptions()
	o.With41 = true
	for seed := uint64(1); seed <= 40; seed++ {
		db := modeltest.Random(seed, o)
		for _, v := range versions {
			fit := Fit(db, v)
			if l := Losses(fit, v); len(l) != 0 {
				t.Fatalf("seed %d, %v: the fitted database still loses %s", seed, v, kinds(l))
			}
			if diff := model.Diff(fit, Fit(fit, v)); len(diff) != 0 {
				t.Fatalf("seed %d, %v: Fit is not idempotent: %v", seed, v, diff)
			}
			if len(Losses(db, v)) == 0 {
				// Nothing is lost: only the unused contents of the pool (and the generator) differ.
				for _, line := range model.Diff(db, fit) {
					if !strings.HasPrefix(line, "Binaries/") {
						t.Fatalf("seed %d, %v: no loss is reported, but Fit changed %s", seed, v, line)
					}
				}
			}
		}
	}
	// The databases of the random generator without 4.1 fields lose nothing in 4.x.
	for seed := uint64(1); seed <= 20; seed++ {
		db := modeltest.Random(seed, modeltest.DefaultOptions())
		if l := Losses(db, Version{4, 0}); len(l) != 0 {
			t.Errorf("seed %d loses %s in 4.0", seed, kinds(l))
		}
	}
}

func TestSupports(t *testing.T) {
	aes := crypto.KDF{ID: crypto.KDFAES, Rounds: 10}
	argon := crypto.KDF{ID: crypto.KDFArgon2id}
	tests := []struct {
		name     string
		version  Version
		settings EncryptionSettings
		route    string // "" when supported
	}{
		{"4.0 with everything", Version{4, 0}, EncryptionSettings{CipherID: crypto.CipherChaCha20, KDF: argon}, ""},
		{"4.1 with AES-KDF", Version{4, 1}, EncryptionSettings{CipherID: crypto.CipherTwofish, KDF: aes}, ""},
		{"3.1 with AES and AES-KDF", Version{3, 1}, EncryptionSettings{CipherID: crypto.CipherAES256, KDF: aes}, ""},
		{"3.0 with Twofish and AES-KDF", Version{3, 0}, EncryptionSettings{CipherID: crypto.CipherTwofish, KDF: aes}, ""},
		{"2.0 with AES and AES-KDF", Version{2, 0}, EncryptionSettings{CipherID: crypto.CipherAES256, KDF: aes}, ""},
		{"3.1 with Argon2", Version{3, 1}, EncryptionSettings{CipherID: crypto.CipherAES256, KDF: argon}, "Supports.EncryptionNotInVersion"},
		{"3.1 with ChaCha20", Version{3, 1}, EncryptionSettings{CipherID: crypto.CipherChaCha20, KDF: aes}, "Supports.EncryptionNotInVersion"},
		{"2.0 with an unknown cipher", Version{2, 0}, EncryptionSettings{CipherID: model.UUID{1}, KDF: aes}, "Supports.EncryptionNotInVersion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Supports(tt.version, tt.settings)
			if tt.route == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			assertRoute(t, err, tt.route)
		})
	}
}
