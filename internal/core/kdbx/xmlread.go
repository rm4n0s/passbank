package kdbx

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/kdbx/crypto"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// Limits on the XML, so that a hostile file cannot take all the memory or the
// stack (SPEC §14).
const (
	// maxGroupDepth is how deep groups may nest.
	maxGroupDepth = 1000
	// maxElementDepth is how deep any elements may nest; it leaves room for the
	// elements around a group at the deepest nesting.
	maxElementDepth = maxGroupDepth + 100
	// maxStringValue is the largest text of one element: 256 MiB.
	maxStringValue = 256 << 20
	// cancelEvery is how many tokens the reader reads between looks at ctx.
	cancelEvery = 1024
)

// secondsToUnixEpoch is the number of seconds from 0001-01-01 to 1970-01-01, and
// maxKDBXSeconds the number to the end of 9999-12-31, the last time KeePass has.
const (
	secondsToUnixEpoch = 62135596800
	maxKDBXSeconds     = 315537897599
)

// xmlReader reads the XML of a KDBX file into the data model.
type xmlReader struct {
	ctx    context.Context
	dec    *xml.Decoder
	stream crypto.InnerStream
	// pool holds the binaries that entries can refer to: those of the inner header
	// of KDBX 4 by index, those of Meta/Binaries of KDBX 2.x and 3.x by ID, and
	// used marks those that an entry refers to.
	pool map[int]poolItem
	used map[int]bool
	// inline numbers the binaries that KDBX 3 entries hold inside the entry (they
	// get negative IDs).
	inline int
	// v3 says that the file is KDBX 2.x or 3.x; headerHash is then the SHA-256 of
	// the header, which Meta/HeaderHash must repeat, and sawHeaderHash says that it
	// did.
	v3            bool
	headerHash    []byte
	sawHeaderHash bool
	tokens        int
	depth         int
	// maxText is the largest text of one element (maxStringValue; smaller in tests).
	maxText int
}

// poolItem is a binary that entries can refer to.
type poolItem struct {
	key  model.BinaryKey
	data secret.Bytes
}

// parseXML reads the XML of a container, of KDBX 4 or of KDBX 2.x and 3.x, into
// a database. The protected values are decrypted with the inner stream in
// document order.
func parseXML(ctx context.Context, c *container) (*model.Database, error) {
	return parseXMLLimit(ctx, c, maxStringValue)
}

// parseXMLLimit is parseXML with another limit for the text of one element, so
// that a test can reach the limit without a 256 MiB file.
func parseXMLLimit(ctx context.Context, c *container, maxText int) (*model.Database, error) {
	stream, err := crypto.NewInnerStream(c.InnerStreamID, c.InnerStreamKey.Reveal())
	if err != nil {
		return nil, err
	}
	r := &xmlReader{
		ctx: ctx, dec: xml.NewDecoder(bytes.NewReader(c.XML)), stream: stream, maxText: maxText,
		pool: map[int]poolItem{}, used: map[int]bool{}, v3: c.Header.Version.Major < 4, headerHash: c.HeaderHash,
	}
	db := &model.Database{Binaries: map[model.BinaryKey]*model.Binary{}}
	defaultMeta(&db.Meta)
	for i, b := range c.Binaries {
		r.pool[i] = poolItem{key: model.BinaryKey{Hash: sha256.Sum256(b.Data.Reveal()), Protected: b.Protected}, data: b.Data}
	}

	err = r.document(db)
	if err == nil && r.v3 && !r.sawHeaderHash && c.Header.Version.Major == 3 && c.Header.Version.Minor >= 1 {
		err = errors.New("FileCorrupt", "the database has no HeaderHash, which a KDBX 3.1 file must have", "where", "Meta")
	}
	if err != nil {
		r.wipePool(c)
		db.Wipe()
		return nil, err
	}
	for i, item := range r.pool {
		if !r.used[i] {
			continue
		}
		if _, ok := db.Binaries[item.key]; !ok {
			db.Binaries[item.key] = &model.Binary{Data: item.data.Clone()}
		}
	}
	r.wipePool(c)
	db.PublicCustomData = append([]model.PublicCustomDataItem(nil), c.Header.PublicCustomData...)
	return db, nil
}

// wipePool wipes the binaries that the reader made itself (those of Meta/Binaries
// and of entries); the container wipes its own.
func (r *xmlReader) wipePool(c *container) {
	if !r.v3 {
		return
	}
	for _, item := range r.pool {
		item.data.Wipe()
	}
}

// next returns the next token, looking at ctx now and then.
func (r *xmlReader) next() (xml.Token, error) {
	r.tokens++
	if r.tokens%cancelEvery == 0 {
		if err := r.ctx.Err(); err != nil {
			return nil, errors.NewErr("ReadCancelled", err)
		}
	}
	tok, err := r.dec.Token()
	if err == io.EOF {
		return nil, errors.New("FileCorrupt", "the XML ends inside an element", "where", "XML")
	}
	if err != nil {
		return nil, errors.NewErr("FileCorrupt", err).SetMetadata("where", "XML")
	}
	return tok, nil
}

// document reads the whole document: the KeePassFile element.
func (r *xmlReader) document(db *model.Database) error {
	for {
		tok, err := r.next()
		if err != nil {
			return err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local != "KeePassFile" {
			return errors.New("FileCorrupt", "the XML is not a KeePass database: its first element is not KeePassFile", "where", "XML")
		}
		err = r.object(se, &db.Raw, nil, func(c xml.StartElement) (bool, error) {
			switch c.Name.Local {
			case "Meta":
				return true, r.meta(c, &db.Meta)
			case "Root":
				return true, r.root(c, db)
			}
			return false, nil
		})
		if err != nil {
			return err
		}
		if db.Root == nil {
			return errors.New("FileCorrupt", "the database has no root group", "where", "XML")
		}
		return nil
	}
}

// children calls f for each child element of the element that has just started,
// and returns after the end of that element. f must read its child to the end.
func (r *xmlReader) children(f func(c xml.StartElement) error) error {
	r.depth++
	defer func() { r.depth-- }()
	if r.depth > maxElementDepth {
		return errors.New("FileCorrupt", "the XML nests elements too deeply", "where", "XML", "limit", maxElementDepth)
	}
	for {
		tok, err := r.next()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if err := f(t); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// object reads an element that the model has fields for. leaf returns the setter
// of a child element that holds only text, or nil when the child is not one;
// nested reads the child elements that hold other elements and says whether it
// knows the child. Anything else goes to raw, or is dropped when raw is nil (the
// small objects of the model have nowhere to keep it).
func (r *xmlReader) object(se xml.StartElement, raw *model.Raw, leaf func(name string) func(string) error, nested func(c xml.StartElement) (bool, error)) error {
	if raw != nil {
		raw.Attrs = rawAttrs(se.Attr)
	}
	return r.children(func(c xml.StartElement) error {
		if leaf != nil {
			// An element with attributes is kept as it is, so that no attribute is lost.
			if set := leaf(c.Name.Local); set != nil && len(c.Attr) == 0 {
				s, err := r.text(c)
				if err != nil {
					return err
				}
				return set(s)
			}
		}
		if nested != nil {
			if handled, err := nested(c); handled || err != nil {
				return err
			}
		}
		if raw == nil {
			return r.skip()
		}
		n, err := r.rawNode(c)
		if err != nil {
			return err
		}
		raw.Nodes = append(raw.Nodes, n)
		return nil
	})
}

// skip reads to the end of the element that has just started, without keeping it.
func (r *xmlReader) skip() error {
	return r.children(func(xml.StartElement) error { return r.skip() })
}

// rawNode reads an element that the model does not know, whole.
func (r *xmlReader) rawNode(se xml.StartElement) (*model.RawNode, error) {
	n := &model.RawNode{Name: se.Name.Local, Attrs: rawAttrs(se.Attr)}
	var text bytes.Buffer
	r.depth++
	defer func() { r.depth-- }()
	if r.depth > maxElementDepth {
		return nil, errors.New("FileCorrupt", "the XML nests elements too deeply", "where", "XML", "limit", maxElementDepth)
	}
	for {
		tok, err := r.next()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			child, err := r.rawNode(t)
			if err != nil {
				return nil, err
			}
			n.Children = append(n.Children, child)
		case xml.CharData:
			if text.Len()+len(t) > r.maxText {
				return nil, errors.New("FileCorrupt", "an XML text is too large", "where", se.Name.Local, "limit", r.maxText)
			}
			text.Write(t)
		case xml.EndElement:
			// Text between child elements is layout, not content.
			if len(n.Children) > 0 && len(bytes.TrimSpace(text.Bytes())) == 0 {
				text.Reset()
			}
			n.Text = secret.New(append([]byte(nil), text.Bytes()...))
			return n, nil
		}
	}
}

func rawAttrs(attrs []xml.Attr) []model.RawAttr {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]model.RawAttr, len(attrs))
	for i, a := range attrs {
		out[i] = model.RawAttr{Name: a.Name.Local, Value: a.Value}
	}
	return out
}

// textBytes reads the text of an element that has no child elements.
func (r *xmlReader) textBytes(se xml.StartElement) ([]byte, error) {
	var text []byte
	for {
		tok, err := r.next()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.CharData:
			if len(text)+len(t) > r.maxText {
				clear(text)
				return nil, errors.New("FileCorrupt", "an XML text is too large", "where", se.Name.Local, "limit", r.maxText)
			}
			text = append(text, t...)
		case xml.StartElement:
			clear(text)
			return nil, errors.New("FileCorrupt", "an element that holds text holds an element", "where", se.Name.Local, "inner", t.Name.Local)
		case xml.EndElement:
			return text, nil
		}
	}
}

func (r *xmlReader) text(se xml.StartElement) (string, error) {
	b, err := r.textBytes(se)
	return string(b), err
}

// xmlValue is what a Value element holds.
type xmlValue struct {
	// data is the text of the element, decrypted when it was protected by the
	// inner stream.
	data secret.Bytes
	// protected is the flag to keep with the field: Protected, or ProtectInMemory
	// (KeePass's own export: plain text that clients mask).
	protected bool
	// decrypted says that data was decrypted, and so is not base64 any more.
	decrypted bool
	// ref is the Ref attribute, -1 when there is none; compressed is the
	// Compressed attribute of an attachment inside an entry (KDBX 3).
	ref        int
	compressed bool
}

// value reads a Value element: the text of a field, or the reference to an
// attachment. A protected value is decrypted with the inner stream.
func (r *xmlReader) value(se xml.StartElement) (xmlValue, error) {
	v := xmlValue{ref: -1}
	for _, a := range se.Attr {
		switch a.Name.Local {
		case "Protected":
			v.decrypted = strings.EqualFold(a.Value, "true")
			v.protected = v.protected || v.decrypted
		case "ProtectInMemory":
			v.protected = v.protected || strings.EqualFold(a.Value, "true")
		case "Compressed":
			v.compressed = strings.EqualFold(a.Value, "true")
		case "Ref":
			n, err := strconv.Atoi(a.Value)
			if err != nil || n < 0 {
				return xmlValue{}, errors.New("FileCorrupt", "an attachment reference is not a number", "where", "Value")
			}
			v.ref = n
		}
	}
	text, err := r.textBytes(se)
	if err != nil {
		return xmlValue{}, err
	}
	if !v.decrypted {
		v.data = secret.New(text)
		return v, nil
	}
	defer clear(text)
	plain := make([]byte, base64.StdEncoding.DecodedLen(len(text)))
	n, err := base64.StdEncoding.Decode(plain, text)
	if err != nil {
		clear(plain)
		return xmlValue{}, errors.New("FileCorrupt", "a protected value is not base64", "where", "Value")
	}
	plain = plain[:n]
	r.stream.XORKeyStream(plain, plain)
	v.data = secret.New(plain)
	return v, nil
}

// defaultMeta sets what KeePass assumes for the settings that a file leaves out.
func defaultMeta(m *model.Meta) {
	m.MaintenanceHistoryDays = 365
	m.MasterKeyChangeRec, m.MasterKeyChangeForce = -1, -1
	m.HistoryMaxItems, m.HistoryMaxSize = 10, 6<<20
	m.RecycleBinEnabled = true
	m.MemoryProtection.Password = true
}

// meta reads the Meta element.
func (r *xmlReader) meta(se xml.StartElement, m *model.Meta) error {
	return r.object(se, &m.Raw, func(name string) func(string) error {
		switch name {
		case "Generator":
			return setString(&m.Generator)
		case "HeaderHash":
			if r.v3 {
				return r.setHeaderHash
			}
		case "DatabaseName":
			return setString(&m.DatabaseName)
		case "DatabaseNameChanged":
			return setTime(name, &m.DatabaseNameChanged)
		case "DatabaseDescription":
			return setString(&m.Description)
		case "DatabaseDescriptionChanged":
			return setTime(name, &m.DescriptionChanged)
		case "DefaultUserName":
			return setString(&m.DefaultUserName)
		case "DefaultUserNameChanged":
			return setTime(name, &m.DefaultUserNameChanged)
		case "SettingsChanged":
			return setTime(name, &m.SettingsChanged)
		case "MaintenanceHistoryDays":
			return setInt(name, &m.MaintenanceHistoryDays)
		case "Color":
			return setString(&m.Color)
		case "MasterKeyChanged":
			return setTime(name, &m.MasterKeyChanged)
		case "MasterKeyChangeRec":
			return setInt64(name, &m.MasterKeyChangeRec)
		case "MasterKeyChangeForce":
			return setInt64(name, &m.MasterKeyChangeForce)
		case "RecycleBinEnabled":
			return setBool(name, &m.RecycleBinEnabled)
		case "RecycleBinUUID":
			return setUUID(name, &m.RecycleBinUUID)
		case "RecycleBinChanged":
			return setTime(name, &m.RecycleBinChanged)
		case "EntryTemplatesGroup":
			return setUUID(name, &m.EntryTemplatesGroup)
		case "EntryTemplatesGroupChanged":
			return setTime(name, &m.EntryTemplatesGroupChanged)
		case "HistoryMaxItems":
			return setInt(name, &m.HistoryMaxItems)
		case "HistoryMaxSize":
			return setInt64(name, &m.HistoryMaxSize)
		case "LastSelectedGroup":
			return setUUID(name, &m.LastSelectedGroup)
		case "LastTopVisibleGroup":
			return setUUID(name, &m.LastTopVisibleGroup)
		}
		return nil
	}, func(c xml.StartElement) (bool, error) {
		switch c.Name.Local {
		case "MemoryProtection":
			return true, r.memoryProtection(c, &m.MemoryProtection)
		case "CustomIcons":
			return true, r.customIcons(c, m)
		case "CustomData":
			return true, r.customData(c, &m.CustomData)
		case "Binaries":
			if r.v3 {
				return true, r.binaries(c)
			}
		}
		return false, nil
	})
}

func (r *xmlReader) memoryProtection(se xml.StartElement, p *model.MemoryProtection) error {
	return r.object(se, nil, func(name string) func(string) error {
		switch name {
		case "ProtectTitle":
			return setBool(name, &p.Title)
		case "ProtectUserName":
			return setBool(name, &p.UserName)
		case "ProtectPassword":
			return setBool(name, &p.Password)
		case "ProtectURL":
			return setBool(name, &p.URL)
		case "ProtectNotes":
			return setBool(name, &p.Notes)
		}
		return nil
	}, nil)
}

func (r *xmlReader) customIcons(se xml.StartElement, m *model.Meta) error {
	return r.object(se, nil, nil, func(c xml.StartElement) (bool, error) {
		if c.Name.Local != "Icon" {
			return false, nil
		}
		var icon model.CustomIcon
		err := r.object(c, nil, func(name string) func(string) error {
			switch name {
			case "UUID":
				return setUUID(name, &icon.UUID)
			case "Data":
				return func(s string) error {
					data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
					if err != nil {
						return errors.New("FileCorrupt", "the data of an icon is not base64", "where", "Icon/Data")
					}
					icon.Data = data
					return nil
				}
			case "Name":
				return setString(&icon.Name)
			case "LastModificationTime":
				return setTime(name, &icon.LastModified)
			}
			return nil
		}, nil)
		if err != nil {
			return true, err
		}
		m.CustomIcons = append(m.CustomIcons, icon)
		return true, nil
	})
}

func (r *xmlReader) customData(se xml.StartElement, out *[]model.CustomDataItem) error {
	return r.object(se, nil, nil, func(c xml.StartElement) (bool, error) {
		if c.Name.Local != "Item" {
			return false, nil
		}
		var item model.CustomDataItem
		err := r.object(c, nil, func(name string) func(string) error {
			switch name {
			case "Key":
				return setString(&item.Key)
			case "Value":
				return setString(&item.Value)
			case "LastModificationTime":
				return setTime(name, &item.LastModified)
			}
			return nil
		}, nil)
		if err != nil {
			return true, err
		}
		*out = append(*out, item)
		return true, nil
	})
}

// root reads the Root element: the root group and the deleted objects.
func (r *xmlReader) root(se xml.StartElement, db *model.Database) error {
	return r.object(se, nil, nil, func(c xml.StartElement) (bool, error) {
		switch c.Name.Local {
		case "Group":
			if db.Root != nil {
				return true, errors.New("FileCorrupt", "the database has more than one root group", "where", "Root")
			}
			g, err := r.group(c, 1)
			if err != nil {
				return true, err
			}
			db.Root = g
			return true, nil
		case "DeletedObjects":
			return true, r.deletedObjects(c, db)
		}
		return false, nil
	})
}

func (r *xmlReader) deletedObjects(se xml.StartElement, db *model.Database) error {
	return r.object(se, nil, nil, func(c xml.StartElement) (bool, error) {
		if c.Name.Local != "DeletedObject" {
			return false, nil
		}
		var d model.DeletedObject
		err := r.object(c, nil, func(name string) func(string) error {
			switch name {
			case "UUID":
				return setUUID(name, &d.UUID)
			case "DeletionTime":
				return setTime(name, &d.DeletionTime)
			}
			return nil
		}, nil)
		if err != nil {
			return true, err
		}
		db.DeletedObjects = append(db.DeletedObjects, d)
		return true, nil
	})
}

// group reads a Group element at the given nesting depth.
func (r *xmlReader) group(se xml.StartElement, depth int) (*model.Group, error) {
	if depth > maxGroupDepth {
		return nil, errors.New("FileCorrupt", "groups nest too deeply", "where", "Group", "limit", maxGroupDepth)
	}
	g := &model.Group{}
	err := r.object(se, &g.Raw, func(name string) func(string) error {
		switch name {
		case "UUID":
			return setUUID(name, &g.UUID)
		case "Name":
			return setString(&g.Name)
		case "Notes":
			return setString(&g.Notes)
		case "Tags":
			return setTags(&g.Tags)
		case "IconID":
			return setInt(name, &g.IconID)
		case "CustomIconUUID":
			return setUUID(name, &g.CustomIcon)
		case "IsExpanded":
			return setBool(name, &g.IsExpanded)
		case "DefaultAutoTypeSequence":
			return setString(&g.DefaultAutoTypeSequence)
		case "EnableAutoType":
			return setTriState(name, &g.EnableAutoType)
		case "EnableSearching":
			return setTriState(name, &g.EnableSearching)
		case "LastTopVisibleEntry":
			return setUUID(name, &g.LastTopVisibleEntry)
		case "PreviousParentGroup":
			return setUUID(name, &g.PreviousParent)
		}
		return nil
	}, func(c xml.StartElement) (bool, error) {
		switch c.Name.Local {
		case "Times":
			return true, r.times(c, &g.Times)
		case "CustomData":
			return true, r.customData(c, &g.CustomData)
		case "Group":
			child, err := r.group(c, depth+1)
			if err != nil {
				return true, err
			}
			g.Groups = append(g.Groups, child)
		case "Entry":
			e, err := r.entry(c, false)
			if err != nil {
				return true, err
			}
			g.Entries = append(g.Entries, e)
		default:
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if g.UUID.IsZero() {
		return nil, errors.New("FileCorrupt", "a group has no UUID", "where", "Group")
	}
	return g, nil
}

// entry reads an Entry element; an entry of a history has no history of its own.
func (r *xmlReader) entry(se xml.StartElement, inHistory bool) (*model.Entry, error) {
	e := &model.Entry{}
	err := r.object(se, &e.Raw, func(name string) func(string) error {
		switch name {
		case "UUID":
			return setUUID(name, &e.UUID)
		case "IconID":
			return setInt(name, &e.IconID)
		case "CustomIconUUID":
			return setUUID(name, &e.CustomIcon)
		case "ForegroundColor":
			return setString(&e.ForegroundColor)
		case "BackgroundColor":
			return setString(&e.BackgroundColor)
		case "OverrideURL":
			return setString(&e.OverrideURL)
		case "Tags":
			return setTags(&e.Tags)
		case "QualityCheck":
			return func(s string) error {
				var b bool
				if err := setBool(name, &b)(s); err != nil {
					return err
				}
				e.QualityCheck = &b
				return nil
			}
		case "PreviousParentGroup":
			return setUUID(name, &e.PreviousParent)
		}
		return nil
	}, func(c xml.StartElement) (bool, error) {
		switch c.Name.Local {
		case "Times":
			return true, r.times(c, &e.Times)
		case "String":
			f, err := r.stringField(c)
			if err != nil {
				return true, err
			}
			e.Fields = append(e.Fields, f)
		case "Binary":
			a, err := r.attachment(c)
			if err != nil {
				return true, err
			}
			e.Attachments = append(e.Attachments, a)
		case "AutoType":
			return true, r.autoType(c, &e.AutoType)
		case "CustomData":
			return true, r.customData(c, &e.CustomData)
		case "History":
			if inHistory {
				return false, nil
			}
			return true, r.history(c, e)
		default:
			return false, nil
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	if e.UUID.IsZero() {
		return nil, errors.New("FileCorrupt", "an entry has no UUID", "where", "Entry")
	}
	return e, nil
}

func (r *xmlReader) history(se xml.StartElement, e *model.Entry) error {
	return r.object(se, nil, nil, func(c xml.StartElement) (bool, error) {
		if c.Name.Local != "Entry" {
			return false, nil
		}
		h, err := r.entry(c, true)
		if err != nil {
			return true, err
		}
		e.History = append(e.History, h)
		return true, nil
	})
}

// stringField reads a String element: a key and a value.
func (r *xmlReader) stringField(se xml.StartElement) (model.Field, error) {
	var f model.Field
	err := r.object(se, nil, func(name string) func(string) error {
		if name == "Key" {
			return setString(&f.Key)
		}
		return nil
	}, func(c xml.StartElement) (bool, error) {
		if c.Name.Local != "Value" {
			return false, nil
		}
		v, err := r.value(c)
		if err != nil {
			return true, err
		}
		f.Value, f.Protected = v.data, v.protected
		return true, nil
	})
	return f, err
}

// attachment reads a Binary element of an entry: a name and a reference to the
// binaries of the inner header (KDBX 4) or of Meta/Binaries (KDBX 3). An entry
// of KDBX 3 may also hold the content inside the entry.
func (r *xmlReader) attachment(se xml.StartElement) (model.Attachment, error) {
	var a model.Attachment
	ref := -1
	err := r.object(se, nil, func(name string) func(string) error {
		if name == "Key" {
			return setString(&a.Name)
		}
		return nil
	}, func(c xml.StartElement) (bool, error) {
		if c.Name.Local != "Value" {
			return false, nil
		}
		v, err := r.value(c)
		if err != nil {
			return true, err
		}
		defer v.data.Wipe()
		ref = v.ref
		if ref < 0 && r.v3 {
			ref, err = r.addInline(v)
		}
		return true, err
	})
	if err != nil {
		return a, err
	}
	item, ok := r.pool[ref]
	if !ok {
		return a, errors.New("FileCorrupt", "an attachment refers to a binary that the file does not have", "where", "Binary", "ref", ref, "binaries", len(r.pool))
	}
	r.used[ref] = true
	a.Binary = item.key
	return a, nil
}

// addInline puts the content that an entry of KDBX 3 holds inside itself into the
// pool, and returns its ID.
func (r *xmlReader) addInline(v xmlValue) (int, error) {
	data := v.data.Reveal()
	if !v.decrypted {
		var err error
		if data, err = decodeBinary(data, v.compressed, r.maxText); err != nil {
			return 0, err
		}
	} else if v.compressed {
		var err error
		if data, err = gunzip(data, r.maxText); err != nil {
			return 0, err
		}
	} else {
		data = append([]byte(nil), data...)
	}
	r.inline--
	r.pool[r.inline] = poolItem{key: model.BinaryKey{Hash: sha256.Sum256(data), Protected: v.protected}, data: secret.New(data)}
	return r.inline, nil
}

// decodeBinary decodes the base64 text of a binary and, when compressed, its
// GZip.
func decodeBinary(text []byte, compressed bool, max int) ([]byte, error) {
	data := make([]byte, base64.StdEncoding.DecodedLen(len(text)))
	n, err := base64.StdEncoding.Decode(data, text)
	if err != nil {
		return nil, errors.New("FileCorrupt", "the content of an attachment is not base64", "where", "Binary")
	}
	data = data[:n]
	if compressed && len(data) > 0 {
		return gunzip(data, max)
	}
	return data, nil
}

// binaries reads Meta/Binaries of KDBX 3: the contents that entries refer to by
// ID. Each is base64, protected by the inner stream when it says so (decrypted
// in document order with the other protected values), and GZip when compressed.
func (r *xmlReader) binaries(se xml.StartElement) error {
	return r.object(se, nil, nil, func(c xml.StartElement) (bool, error) {
		if c.Name.Local != "Binary" {
			return false, nil
		}
		id, compressed, protected := -1, false, false
		for _, at := range c.Attr {
			switch at.Name.Local {
			case "ID":
				n, err := strconv.Atoi(at.Value)
				if err != nil || n < 0 {
					return true, errors.New("FileCorrupt", "the ID of a binary is not a number", "where", "Binaries")
				}
				id = n
			case "Compressed":
				compressed = strings.EqualFold(at.Value, "true")
			case "Protected":
				protected = strings.EqualFold(at.Value, "true")
			}
		}
		if id < 0 {
			return true, errors.New("FileCorrupt", "a binary has no ID", "where", "Binaries")
		}
		if _, dup := r.pool[id]; dup {
			return true, errors.New("FileCorrupt", "two binaries have the same ID", "where", "Binaries", "id", id)
		}
		text, err := r.textBytes(c)
		if err != nil {
			return true, err
		}
		defer clear(text)
		data := make([]byte, base64.StdEncoding.DecodedLen(len(text)))
		n, err := base64.StdEncoding.Decode(data, text)
		if err != nil {
			return true, errors.New("FileCorrupt", "the content of a binary is not base64", "where", "Binaries", "id", id)
		}
		data = data[:n]
		if protected {
			r.stream.XORKeyStream(data, data)
		}
		if compressed && len(data) > 0 {
			plain, err := gunzip(data, r.maxText)
			clear(data)
			if err != nil {
				return true, err
			}
			data = plain
		}
		r.pool[id] = poolItem{key: model.BinaryKey{Hash: sha256.Sum256(data), Protected: protected}, data: secret.New(data)}
		return true, nil
	})
}

// setHeaderHash is the setter of Meta/HeaderHash in a KDBX 3 file: its value
// must be the SHA-256 of the header.
func (r *xmlReader) setHeaderHash(s string) error {
	stored, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return errors.New("FileCorrupt", "the HeaderHash is not base64", "where", "Meta")
	}
	if !bytes.Equal(stored, r.headerHash) {
		return errors.New("FileCorrupt", "the HeaderHash does not match the header: the file is damaged", "where", "Meta")
	}
	r.sawHeaderHash = true
	return nil
}

func (r *xmlReader) autoType(se xml.StartElement, a *model.AutoType) error {
	return r.object(se, nil, func(name string) func(string) error {
		switch name {
		case "Enabled":
			return setBool(name, &a.Enabled)
		case "DataTransferObfuscation":
			return setInt(name, &a.DataTransferObfuscation)
		case "DefaultSequence":
			return setString(&a.DefaultSequence)
		}
		return nil
	}, func(c xml.StartElement) (bool, error) {
		if c.Name.Local != "Association" {
			return false, nil
		}
		var assoc model.AutoTypeAssociation
		err := r.object(c, nil, func(name string) func(string) error {
			switch name {
			case "Window":
				return setString(&assoc.Window)
			case "KeystrokeSequence":
				return setString(&assoc.Sequence)
			}
			return nil
		}, nil)
		if err != nil {
			return true, err
		}
		a.Associations = append(a.Associations, assoc)
		return true, nil
	})
}

func (r *xmlReader) times(se xml.StartElement, t *model.Times) error {
	return r.object(se, nil, func(name string) func(string) error {
		switch name {
		case "CreationTime":
			return setTime(name, &t.Creation)
		case "LastModificationTime":
			return setTime(name, &t.LastModification)
		case "LastAccessTime":
			return setTime(name, &t.LastAccess)
		case "ExpiryTime":
			return setTime(name, &t.Expiry)
		case "Expires":
			return setBool(name, &t.Expires)
		case "UsageCount":
			return setInt64(name, &t.UsageCount)
		case "LocationChanged":
			return setTime(name, &t.LocationChanged)
		}
		return nil
	}, nil)
}

// The setters turn the text of an element into the value of a field.

func setString(p *string) func(string) error {
	return func(s string) error { *p = s; return nil }
}

func setInt(name string, p *int) func(string) error {
	return func(s string) error {
		n, err := parseXMLInt(name, s)
		*p = int(n)
		return err
	}
}

func setInt64(name string, p *int64) func(string) error {
	return func(s string) error {
		n, err := parseXMLInt(name, s)
		*p = n
		return err
	}
}

func setBool(name string, p *bool) func(string) error {
	return func(s string) error {
		b, err := parseXMLBool(name, s)
		*p = b
		return err
	}
}

func setUUID(name string, p *model.UUID) func(string) error {
	return func(s string) error {
		u, err := parseXMLUUID(name, s)
		*p = u
		return err
	}
}

func setTime(name string, p *time.Time) func(string) error {
	return func(s string) error {
		t, err := parseXMLTime(name, s)
		*p = t
		return err
	}
}

func setTriState(name string, p *model.TriState) func(string) error {
	return func(s string) error {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "", "null":
			*p = model.Inherit
		case "true":
			*p = model.Enabled
		case "false":
			*p = model.Disabled
		default:
			return errors.New("FileCorrupt", "an element is not true, false or null", "where", name)
		}
		return nil
	}
}

// setTags splits the tags at ";" and ",", as KeePass does.
func setTags(p *[]string) func(string) error {
	return func(s string) error {
		var tags []string
		for _, t := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' }) {
			if t = strings.TrimSpace(t); t != "" {
				tags = append(tags, t)
			}
		}
		*p = tags
		return nil
	}
}

func parseXMLInt(name, s string) (int64, error) {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0, errors.New("FileCorrupt", "an element is not a whole number", "where", name)
	}
	return n, nil
}

func parseXMLBool(name, s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true":
		return true, nil
	case "false", "":
		return false, nil
	}
	return false, errors.New("FileCorrupt", "an element is not True or False", "where", name)
}

// parseXMLUUID reads a UUID as base64; an empty text is the zero UUID.
func parseXMLUUID(name, s string) (model.UUID, error) {
	var u model.UUID
	s = strings.TrimSpace(s)
	if s == "" {
		return u, nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != len(u) {
		return u, errors.New("FileCorrupt", "an element is not a UUID", "where", name)
	}
	copy(u[:], b)
	return u, nil
}

// parseXMLTime reads a time in either form that KDBX has: ISO 8601 (always has "-" and ":", which base64 has not; KDBX 3, and
// files that KeePass 2.x or other programs wrote as KDBX 4) or base64 of the
// seconds since 0001-01-01 (KDBX 4). The result is UTC with whole seconds.
func parseXMLTime(name, s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	if strings.ContainsAny(s, ":-") {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}, errors.New("FileCorrupt", "an element is not a time", "where", name)
		}
		return t.UTC().Truncate(time.Second), nil
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != 8 {
		return time.Time{}, errors.New("FileCorrupt", "an element is not a time", "where", name)
	}
	secs := int64(binary.LittleEndian.Uint64(b))
	if secs < 0 || secs > maxKDBXSeconds {
		return time.Time{}, errors.New("FileCorrupt", "a time is out of range", "where", name)
	}
	return time.Unix(secs-secondsToUnixEpoch, 0).UTC(), nil
}
