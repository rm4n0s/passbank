package kdbx

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"strconv"
	"strings"
	"time"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
)

// xmlSpec says how the XML of a database is written.
type xmlSpec struct {
	// Version is the KDBX version of the file: the 4.1 elements are written for
	// 4.1 only, and KDBX 2.x and 3.x get their dialect (ISO times, Meta/Binaries,
	// HeaderHash).
	Version Version
	// Stream is the inner stream that protects values; it is used in document
	// order and must be new.
	Stream crypto.InnerStream
	// HeaderHash is the SHA-256 of the header, written as Meta/HeaderHash in KDBX 2.x
	// and 3.x.
	HeaderHash []byte
	// CompressBinaries compresses the attachments of Meta/Binaries (KDBX 2.x and
	// 3.x); the container's compression decides it.
	CompressBinaries bool
}

func (s xmlSpec) v3() bool     { return s.Version.Major < 4 }
func (s xmlSpec) v41() bool    { return s.Version.Major == 4 && s.Version.Minor >= 1 }
func (s xmlSpec) v4plus() bool { return s.Version.Major >= 4 }

// MinVersion returns the lowest KDBX 4 version that holds everything in the
// database: 4.1 when it has a field that only 4.1 stores (SPEC §5), else 4.0.
func MinVersion(db *model.Database) Version {
	if needs41(db) {
		return Version{4, 1}
	}
	return Version{4, 0}
}

// needs41 reports whether the database holds data that only KDBX 4.1 can store.
func needs41(db *model.Database) bool {
	m := &db.Meta
	if !m.SettingsChanged.IsZero() {
		return true
	}
	for _, ic := range m.CustomIcons {
		if ic.Name != "" || !ic.LastModified.IsZero() {
			return true
		}
	}
	custom := func(items []model.CustomDataItem) bool {
		for _, it := range items {
			if !it.LastModified.IsZero() {
				return true
			}
		}
		return false
	}
	if custom(m.CustomData) {
		return true
	}
	found := false
	db.WalkGroups(func(g *model.Group) bool {
		if len(g.Tags) > 0 || !g.PreviousParent.IsZero() || custom(g.CustomData) {
			found = true
			return false
		}
		for _, e := range g.Entries {
			if entryNeeds41(e, custom) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func entryNeeds41(e *model.Entry, custom func([]model.CustomDataItem) bool) bool {
	if e.QualityCheck != nil || !e.PreviousParent.IsZero() || custom(e.CustomData) {
		return true
	}
	for _, h := range e.History {
		if entryNeeds41(h, custom) {
			return true
		}
	}
	return false
}

// xmlWriter writes the XML.
type xmlWriter struct {
	spec xmlSpec
	buf  bytes.Buffer
	// ids maps the contents that entries use to their number: the index in the
	// inner header (KDBX 4) or the ID in Meta/Binaries (KDBX 2.x and 3.x). order lists
	// them by number.
	ids   map[model.BinaryKey]int
	order []model.BinaryKey
	db    *model.Database
}

// marshalXML writes the XML of the database. It returns the XML and, for
// KDBX 4, the contents of the attachments for the inner header, in the order of
// their numbers. The inner stream protects the values in document order.
func marshalXML(db *model.Database, spec xmlSpec) ([]byte, []innerBinaryData, error) {
	if db.Root == nil {
		return nil, nil, errors.New("DatabaseInvalid", "the database has no root group")
	}
	w := &xmlWriter{spec: spec, ids: map[model.BinaryKey]int{}, db: db}
	if err := w.collectBinaries(); err != nil {
		return nil, nil, err
	}
	w.document()
	var binaries []innerBinaryData
	if spec.v4plus() {
		for _, key := range w.order {
			binaries = append(binaries, innerBinaryData{Protected: key.Protected, Data: db.Binaries[key].Data.Clone()})
		}
	}
	return w.buf.Bytes(), binaries, nil
}

// collectBinaries numbers the contents that entries (and their history) use, in
// document order, and checks that the pool has them.
func (w *xmlWriter) collectBinaries() error {
	var err error
	note := func(e *model.Entry) {
		for _, a := range e.Attachments {
			if _, ok := w.ids[a.Binary]; ok {
				continue
			}
			if b := w.db.Binaries[a.Binary]; b == nil {
				err = errors.New("DatabaseInvalid", "an attachment refers to a content that the database does not have", "entry", e.UUID.String())
				return
			}
			w.ids[a.Binary] = len(w.order)
			w.order = append(w.order, a.Binary)
		}
	}
	w.db.WalkEntries(func(_ *model.Group, e *model.Entry) bool {
		note(e)
		for _, h := range e.History {
			note(h)
		}
		return err == nil
	})
	return err
}

// --- low-level output ---

func (w *xmlWriter) raw(s string) { w.buf.WriteString(s) }

// text writes s with the characters that XML reserves replaced.
func (w *xmlWriter) text(s string) {
	_ = xml.EscapeText(&w.buf, []byte(s)) // writing to a bytes.Buffer cannot fail
}

// open writes a start tag with attributes (name, value pairs).
func (w *xmlWriter) open(name string, attrs ...string) {
	w.raw("<" + name)
	for i := 0; i+1 < len(attrs); i += 2 {
		w.raw(" " + attrs[i] + `="`)
		w.text(attrs[i+1])
		w.raw(`"`)
	}
	w.raw(">")
}

func (w *xmlWriter) close(name string) { w.raw("</" + name + ">\n") }

// elem writes an element holding text.
func (w *xmlWriter) elem(name, value string) {
	w.raw("<" + name + ">")
	w.text(value)
	w.raw("</" + name + ">\n")
}

func (w *xmlWriter) boolElem(name string, v bool) { w.elem(name, boolText(v)) }

func boolText(v bool) string {
	if v {
		return "True"
	}
	return "False"
}

func (w *xmlWriter) intElem(name string, v int64) { w.elem(name, strconv.FormatInt(v, 10)) }

func (w *xmlWriter) uuidElem(name string, u model.UUID) {
	w.elem(name, base64.StdEncoding.EncodeToString(u[:]))
}

func (w *xmlWriter) timeElem(name string, t time.Time) { w.elem(name, w.formatTime(t)) }

// formatTime writes a time in the form of the version: ISO 8601 for KDBX 2.x and
// 3.x, base64 of the seconds since 0001-01-01 for KDBX 4.
func (w *xmlWriter) formatTime(t time.Time) string {
	t = t.UTC().Truncate(time.Second)
	if w.spec.v3() {
		return t.Format("2006-01-02T15:04:05Z")
	}
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(t.Unix()+secondsToUnixEpoch))
	return base64.StdEncoding.EncodeToString(b[:])
}

// --- the document ---

func (w *xmlWriter) document() {
	w.raw(`<?xml version="1.0" encoding="utf-8" standalone="yes"?>` + "\n")
	w.open("KeePassFile", rawAttrPairs(w.db.Raw.Attrs)...)
	w.raw("\n")
	w.meta()
	w.open("Root")
	w.raw("\n")
	w.group(w.db.Root)
	w.deletedObjects()
	w.close("Root")
	w.rawNodes(w.db.Raw.Nodes)
	w.close("KeePassFile")
}

func rawAttrPairs(attrs []model.RawAttr) []string {
	out := make([]string, 0, 2*len(attrs))
	for _, a := range attrs {
		out = append(out, a.Name, a.Value)
	}
	return out
}

// rawNodes writes the XML the model does not know, as it was read: without any
// layout, because the text of a node is data.
func (w *xmlWriter) rawNodes(nodes []*model.RawNode) {
	for _, n := range nodes {
		w.rawNode(n)
		w.raw("\n")
	}
}

func (w *xmlWriter) rawNode(n *model.RawNode) {
	w.open(n.Name, rawAttrPairs(n.Attrs)...)
	w.text(string(n.Text.Reveal()))
	for _, c := range n.Children {
		w.rawNode(c)
	}
	w.raw("</" + n.Name + ">")
}

func (w *xmlWriter) meta() {
	m := &w.db.Meta
	w.open("Meta", rawAttrPairs(m.Raw.Attrs)...)
	w.raw("\n")
	generator := m.Generator
	if generator == "" {
		generator = "Passbank"
	}
	w.elem("Generator", generator)
	if w.spec.v3() {
		w.elem("HeaderHash", base64.StdEncoding.EncodeToString(w.spec.HeaderHash))
	}
	if w.spec.v41() {
		w.timeElem("SettingsChanged", m.SettingsChanged)
	}
	w.elem("DatabaseName", m.DatabaseName)
	w.timeElem("DatabaseNameChanged", m.DatabaseNameChanged)
	w.elem("DatabaseDescription", m.Description)
	w.timeElem("DatabaseDescriptionChanged", m.DescriptionChanged)
	w.elem("DefaultUserName", m.DefaultUserName)
	w.timeElem("DefaultUserNameChanged", m.DefaultUserNameChanged)
	w.intElem("MaintenanceHistoryDays", int64(m.MaintenanceHistoryDays))
	w.elem("Color", m.Color)
	w.timeElem("MasterKeyChanged", m.MasterKeyChanged)
	w.intElem("MasterKeyChangeRec", m.MasterKeyChangeRec)
	w.intElem("MasterKeyChangeForce", m.MasterKeyChangeForce)
	w.open("MemoryProtection")
	w.boolElem("ProtectTitle", m.MemoryProtection.Title)
	w.boolElem("ProtectUserName", m.MemoryProtection.UserName)
	w.boolElem("ProtectPassword", m.MemoryProtection.Password)
	w.boolElem("ProtectURL", m.MemoryProtection.URL)
	w.boolElem("ProtectNotes", m.MemoryProtection.Notes)
	w.close("MemoryProtection")
	w.open("CustomIcons")
	for _, ic := range m.CustomIcons {
		w.open("Icon")
		w.uuidElem("UUID", ic.UUID)
		w.elem("Data", base64.StdEncoding.EncodeToString(ic.Data))
		if w.spec.v41() {
			if ic.Name != "" {
				w.elem("Name", ic.Name)
			}
			if !ic.LastModified.IsZero() {
				w.timeElem("LastModificationTime", ic.LastModified)
			}
		}
		w.close("Icon")
	}
	w.close("CustomIcons")
	w.boolElem("RecycleBinEnabled", m.RecycleBinEnabled)
	w.uuidElem("RecycleBinUUID", m.RecycleBinUUID)
	w.timeElem("RecycleBinChanged", m.RecycleBinChanged)
	w.uuidElem("EntryTemplatesGroup", m.EntryTemplatesGroup)
	w.timeElem("EntryTemplatesGroupChanged", m.EntryTemplatesGroupChanged)
	w.intElem("HistoryMaxItems", int64(m.HistoryMaxItems))
	w.intElem("HistoryMaxSize", m.HistoryMaxSize)
	w.uuidElem("LastSelectedGroup", m.LastSelectedGroup)
	w.uuidElem("LastTopVisibleGroup", m.LastTopVisibleGroup)
	if w.spec.v3() {
		w.binaries()
	}
	w.customData(m.CustomData)
	w.rawNodes(m.Raw.Nodes)
	w.close("Meta")
}

// customData writes a CustomData element when there are items. The modification
// time of an item is a KDBX 4.1 field.
func (w *xmlWriter) customData(items []model.CustomDataItem) {
	if len(items) == 0 {
		return
	}
	w.open("CustomData")
	w.raw("\n")
	for _, it := range items {
		w.open("Item")
		w.elem("Key", it.Key)
		w.elem("Value", it.Value)
		if w.spec.v41() && !it.LastModified.IsZero() {
			w.timeElem("LastModificationTime", it.LastModified)
		}
		w.close("Item")
	}
	w.close("CustomData")
}

// binaries writes Meta/Binaries of KDBX 2.x and 3.x: the attachments by ID,
// compressed when asked and protected by the inner stream when their flag says so.
func (w *xmlWriter) binaries() {
	w.open("Binaries")
	w.raw("\n")
	for id, key := range w.order {
		data := w.db.Binaries[key].Data.Reveal()
		attrs := []string{"ID", strconv.Itoa(id)}
		if w.spec.CompressBinaries {
			attrs = append(attrs, "Compressed", "True")
			if len(data) > 0 {
				var buf bytes.Buffer
				zw := gzip.NewWriter(&buf)
				_, _ = zw.Write(data) // writing to a bytes.Buffer cannot fail
				_ = zw.Close()
				data = buf.Bytes()
			} else {
				data = nil
			}
		} else {
			data = append([]byte(nil), data...)
		}
		if key.Protected {
			attrs = append(attrs, "Protected", "True")
			w.spec.Stream.XORKeyStream(data, data)
		}
		w.open("Binary", attrs...)
		w.raw(base64.StdEncoding.EncodeToString(data))
		w.close("Binary")
	}
	w.close("Binaries")
}

func (w *xmlWriter) times(t model.Times) {
	w.open("Times")
	w.raw("\n")
	w.timeElem("CreationTime", t.Creation)
	w.timeElem("LastModificationTime", t.LastModification)
	w.timeElem("LastAccessTime", t.LastAccess)
	w.timeElem("ExpiryTime", t.Expiry)
	w.boolElem("Expires", t.Expires)
	w.intElem("UsageCount", t.UsageCount)
	w.timeElem("LocationChanged", t.LocationChanged)
	w.close("Times")
}

func triText(t model.TriState) string {
	switch t {
	case model.Enabled:
		return "true"
	case model.Disabled:
		return "false"
	}
	return "null"
}

func (w *xmlWriter) group(g *model.Group) {
	w.open("Group", rawAttrPairs(g.Raw.Attrs)...)
	w.raw("\n")
	w.uuidElem("UUID", g.UUID)
	w.elem("Name", g.Name)
	w.elem("Notes", g.Notes)
	w.intElem("IconID", int64(g.IconID))
	if !g.CustomIcon.IsZero() {
		w.uuidElem("CustomIconUUID", g.CustomIcon)
	}
	w.times(g.Times)
	w.boolElem("IsExpanded", g.IsExpanded)
	w.elem("DefaultAutoTypeSequence", g.DefaultAutoTypeSequence)
	w.elem("EnableAutoType", triText(g.EnableAutoType))
	w.elem("EnableSearching", triText(g.EnableSearching))
	w.uuidElem("LastTopVisibleEntry", g.LastTopVisibleEntry)
	if w.spec.v41() {
		if len(g.Tags) > 0 {
			w.elem("Tags", strings.Join(g.Tags, ";"))
		}
		if !g.PreviousParent.IsZero() {
			w.uuidElem("PreviousParentGroup", g.PreviousParent)
		}
	}
	if w.spec.v4plus() {
		w.customData(g.CustomData)
	}
	for _, e := range g.Entries {
		w.entry(e, false)
	}
	for _, c := range g.Groups {
		w.group(c)
	}
	w.rawNodes(g.Raw.Nodes)
	w.close("Group")
}

func (w *xmlWriter) entry(e *model.Entry, inHistory bool) {
	w.open("Entry", rawAttrPairs(e.Raw.Attrs)...)
	w.raw("\n")
	w.uuidElem("UUID", e.UUID)
	w.intElem("IconID", int64(e.IconID))
	if !e.CustomIcon.IsZero() {
		w.uuidElem("CustomIconUUID", e.CustomIcon)
	}
	w.elem("ForegroundColor", e.ForegroundColor)
	w.elem("BackgroundColor", e.BackgroundColor)
	w.elem("OverrideURL", e.OverrideURL)
	w.elem("Tags", strings.Join(e.Tags, ";"))
	if w.spec.v41() {
		if e.QualityCheck != nil {
			w.boolElem("QualityCheck", *e.QualityCheck)
		}
		if !e.PreviousParent.IsZero() {
			w.uuidElem("PreviousParentGroup", e.PreviousParent)
		}
	}
	w.times(e.Times)
	for _, f := range e.Fields {
		w.open("String")
		w.elem("Key", f.Key)
		w.value(f)
		w.close("String")
	}
	for _, a := range e.Attachments {
		w.open("Binary")
		w.elem("Key", a.Name)
		w.raw(`<Value Ref="` + strconv.Itoa(w.ids[a.Binary]) + `"/>` + "\n")
		w.close("Binary")
	}
	w.open("AutoType")
	w.raw("\n")
	w.boolElem("Enabled", e.AutoType.Enabled)
	w.intElem("DataTransferObfuscation", int64(e.AutoType.DataTransferObfuscation))
	w.elem("DefaultSequence", e.AutoType.DefaultSequence)
	for _, as := range e.AutoType.Associations {
		w.open("Association")
		w.elem("Window", as.Window)
		w.elem("KeystrokeSequence", as.Sequence)
		w.close("Association")
	}
	w.close("AutoType")
	if w.spec.v4plus() {
		w.customData(e.CustomData)
	}
	if !inHistory && len(e.History) > 0 {
		w.open("History")
		w.raw("\n")
		for _, h := range e.History {
			w.entry(h, true)
		}
		w.close("History")
	}
	w.rawNodes(e.Raw.Nodes)
	w.close("Entry")
}

// value writes the Value element of a field; a protected value is encrypted with
// the inner stream, which advances in document order.
func (w *xmlWriter) value(f model.Field) {
	if !f.Protected {
		w.elem("Value", f.Text())
		return
	}
	enc := append([]byte(nil), f.Value.Reveal()...)
	w.spec.Stream.XORKeyStream(enc, enc)
	w.raw(`<Value Protected="True">` + base64.StdEncoding.EncodeToString(enc) + "</Value>\n")
	clear(enc)
}

func (w *xmlWriter) deletedObjects() {
	w.open("DeletedObjects")
	w.raw("\n")
	for _, d := range w.db.DeletedObjects {
		w.open("DeletedObject")
		w.uuidElem("UUID", d.UUID)
		w.timeElem("DeletionTime", d.DeletionTime)
		w.close("DeletedObject")
	}
	w.close("DeletedObjects")
}
