// Package modeltest makes random databases for property tests: trees of groups
// and entries with fields, attachments, histories, custom data and unknown XML,
// all bounded in size and fully determined by a seed.
package modeltest

import (
	"crypto/sha256"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/secret"
)

// Options say what a random database holds.
type Options struct {
	// MaxDepth is how deep groups nest below the root (at least 1).
	MaxDepth int
	// MaxGroups and MaxEntries bound the children of a group.
	MaxGroups, MaxEntries int
	// With41 adds the fields that only KDBX 4.1 stores.
	With41 bool
	// WithItemCustomData adds custom data to groups and entries, which KDBX 4
	// stores and KDBX 2.x and 3.x do not.
	WithItemCustomData bool
	// WithRaw adds unknown XML to the database, Meta, groups and entries.
	WithRaw bool
}

// DefaultOptions are small, with everything on except the 4.1 fields.
func DefaultOptions() Options {
	return Options{MaxDepth: 3, MaxGroups: 3, MaxEntries: 4, WithItemCustomData: true, WithRaw: true}
}

type gen struct {
	r        *rand.Rand
	o        Options
	uuidN    uint64
	icons    []model.UUID
	pool     []poolItem
	binaries map[model.BinaryKey]*model.Binary
}

type poolItem struct {
	key model.BinaryKey
}

// Random returns the database of the seed: the same seed and options always give
// the same database.
func Random(seed uint64, o Options) *model.Database {
	if o.MaxDepth < 1 {
		o.MaxDepth = 1
	}
	g := &gen{r: rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)), o: o, binaries: map[model.BinaryKey]*model.Binary{}}
	db := &model.Database{Binaries: g.binaries}
	db.Meta = g.meta()
	for _, ic := range db.Meta.CustomIcons {
		g.icons = append(g.icons, ic.UUID)
	}
	g.makeBinaries()
	db.Root = g.group(0)
	db.Root.Name = "Root"
	for i := g.r.IntN(4); i > 0; i-- {
		db.DeletedObjects = append(db.DeletedObjects, model.DeletedObject{UUID: g.uuid(), DeletionTime: g.time()})
	}
	if o.WithRaw && g.r.IntN(2) == 0 {
		db.Raw = model.Raw{Attrs: g.attrs(), Nodes: g.rawNodes(2)}
	}
	db.PublicCustomData = g.publicCustomData()
	return db
}

func (g *gen) uuid() model.UUID {
	g.uuidN++
	var u model.UUID
	for i := range u {
		u[i] = byte(g.r.Uint32())
	}
	u[0], u[1] = byte(g.uuidN), byte(g.uuidN>>8) // unique within a database
	return u
}

func (g *gen) maybeUUID() model.UUID {
	if g.r.IntN(3) == 0 {
		return model.UUID{}
	}
	return g.uuid()
}

func (g *gen) time() time.Time {
	// Whole seconds between 1970 and 2100, and now and then the zero time.
	if g.r.IntN(10) == 0 {
		return time.Time{}
	}
	return time.Unix(g.r.Int64N(130*365*24*3600), 0).UTC()
}

// alphabet has plain letters, spaces and what XML treats specially, and
// characters from other scripts. It has no control characters except the line
// break, the tab and the carriage return, which XML allows.
var alphabet = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789  _-.:/\\@#%^*()[]{}!?=+~'\"&<>;,|\n\r\täöüßéñ日本語中文ключ😀🔑")

func (g *gen) str(max int) string {
	n := g.r.IntN(max + 1)
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteRune(alphabet[g.r.IntN(len(alphabet))])
	}
	return sb.String()
}

// word is a short string without whitespace or the separators of tags.
func (g *gen) word() string {
	var sb strings.Builder
	for i := 0; i < 1+g.r.IntN(8); i++ {
		sb.WriteRune(alphabet[g.r.IntN(26)])
	}
	return sb.String()
}

func (g *gen) color() string {
	if g.r.IntN(2) == 0 {
		return ""
	}
	const hex = "0123456789ABCDEF"
	b := []byte("#RRGGBB")
	for i := 1; i < 7; i++ {
		b[i] = hex[g.r.IntN(16)]
	}
	return string(b)
}

func (g *gen) secret(s string) secret.Bytes { return secret.New([]byte(s)) }

func (g *gen) bool() bool { return g.r.IntN(2) == 0 }

func (g *gen) meta() model.Meta {
	m := model.Meta{
		Generator: "modeltest", DatabaseName: g.str(20), DatabaseNameChanged: g.time(), Description: g.str(30), DescriptionChanged: g.time(),
		DefaultUserName: g.str(10), DefaultUserNameChanged: g.time(), MaintenanceHistoryDays: g.r.IntN(1000), Color: g.color(),
		MasterKeyChanged: g.time(), MasterKeyChangeRec: g.r.Int64N(1000) - 1, MasterKeyChangeForce: g.r.Int64N(1000) - 1,
		MemoryProtection:  model.MemoryProtection{Title: g.bool(), UserName: g.bool(), Password: g.bool(), URL: g.bool(), Notes: g.bool()},
		RecycleBinEnabled: g.bool(), RecycleBinUUID: g.maybeUUID(), RecycleBinChanged: g.time(),
		EntryTemplatesGroup: g.maybeUUID(), EntryTemplatesGroupChanged: g.time(),
		HistoryMaxItems: g.r.IntN(50) - 1, HistoryMaxSize: g.r.Int64N(1<<24) - 1,
		LastSelectedGroup: g.maybeUUID(), LastTopVisibleGroup: g.maybeUUID(),
	}
	for i := g.r.IntN(3); i > 0; i-- {
		ic := model.CustomIcon{UUID: g.uuid(), Data: []byte(g.str(40))}
		if g.o.With41 {
			if g.bool() {
				ic.Name = g.str(10)
			}
			if g.bool() {
				ic.LastModified = g.time()
			}
		}
		m.CustomIcons = append(m.CustomIcons, ic)
	}
	m.CustomData = g.customData(true)
	if g.o.With41 {
		m.SettingsChanged = g.time()
	}
	if g.o.WithRaw && g.bool() {
		m.Raw = model.Raw{Attrs: g.attrs(), Nodes: g.rawNodes(2)}
	}
	return m
}

func (g *gen) customData(allowed bool) []model.CustomDataItem {
	if !allowed {
		return nil
	}
	var items []model.CustomDataItem
	for i := g.r.IntN(3); i > 0; i-- {
		it := model.CustomDataItem{Key: g.word(), Value: g.str(20)}
		if g.o.With41 && g.bool() {
			it.LastModified = g.time()
		}
		items = append(items, it)
	}
	return items
}

func (g *gen) publicCustomData() []model.PublicCustomDataItem {
	var items []model.PublicCustomDataItem
	for i := g.r.IntN(3); i > 0; i-- {
		switch g.r.IntN(3) {
		case 0:
			items = append(items, model.PublicCustomDataItem{Key: g.word(), Type: 0x04, Data: []byte{1, 2, 3, 4}})
		case 1:
			items = append(items, model.PublicCustomDataItem{Key: g.word(), Type: 0x18, Data: []byte(g.str(10))})
		default:
			items = append(items, model.PublicCustomDataItem{Key: g.word(), Type: 0x42, Data: []byte(g.str(10))})
		}
	}
	return items
}

// makeBinaries makes the pool: a few contents, some empty, some protected.
func (g *gen) makeBinaries() {
	for i := 1 + g.r.IntN(4); i > 0; i-- {
		content := []byte(g.str(60))
		if g.r.IntN(5) == 0 {
			content = nil
		}
		key := model.BinaryKey{Hash: sha256.Sum256(content), Protected: g.bool()}
		if _, dup := g.binaries[key]; dup {
			continue
		}
		g.binaries[key] = &model.Binary{Data: secret.New(content)}
		g.pool = append(g.pool, poolItem{key: key})
	}
}

func (g *gen) times() model.Times {
	return model.Times{Creation: g.time(), LastModification: g.time(), LastAccess: g.time(), Expiry: g.time(), Expires: g.bool(),
		UsageCount: g.r.Int64N(1000), LocationChanged: g.time()}
}

func (g *gen) tri() model.TriState { return model.TriState(g.r.IntN(3)) }

func (g *gen) group(depth int) *model.Group {
	grp := &model.Group{
		UUID: g.uuid(), Name: g.str(15), Notes: g.str(30), IconID: g.r.IntN(70), Times: g.times(), IsExpanded: g.bool(),
		DefaultAutoTypeSequence: g.str(10), EnableAutoType: g.tri(), EnableSearching: g.tri(), LastTopVisibleEntry: g.maybeUUID(),
	}
	if len(g.icons) > 0 && g.r.IntN(3) == 0 {
		grp.CustomIcon = g.icons[g.r.IntN(len(g.icons))]
	}
	if g.o.With41 {
		for i := g.r.IntN(3); i > 0; i-- {
			grp.Tags = append(grp.Tags, g.word())
		}
		grp.PreviousParent = g.maybeUUID()
	}
	grp.CustomData = g.customData(g.o.WithItemCustomData)
	if g.o.WithRaw && g.r.IntN(3) == 0 {
		grp.Raw = model.Raw{Attrs: g.attrs(), Nodes: g.rawNodes(2)}
	}
	for i := g.r.IntN(g.o.MaxEntries + 1); i > 0; i-- {
		grp.Entries = append(grp.Entries, g.entry(true))
	}
	if depth < g.o.MaxDepth {
		for i := g.r.IntN(g.o.MaxGroups + 1); i > 0; i-- {
			grp.Groups = append(grp.Groups, g.group(depth+1))
		}
	}
	return grp
}

func (g *gen) entry(withHistory bool) *model.Entry {
	e := &model.Entry{
		UUID: g.uuid(), IconID: g.r.IntN(70), ForegroundColor: g.color(), BackgroundColor: g.color(), OverrideURL: g.str(10),
		Times: g.times(),
	}
	if len(g.icons) > 0 && g.r.IntN(3) == 0 {
		e.CustomIcon = g.icons[g.r.IntN(len(g.icons))]
	}
	for i := g.r.IntN(4); i > 0; i-- {
		e.Tags = append(e.Tags, g.word())
	}
	for _, key := range []string{model.FieldTitle, model.FieldUserName, model.FieldPassword, model.FieldURL, model.FieldNotes} {
		e.Fields = append(e.Fields, model.Field{Key: key, Value: g.secret(g.str(25)), Protected: key == model.FieldPassword || g.r.IntN(6) == 0})
	}
	for i := g.r.IntN(4); i > 0; i-- {
		e.Fields = append(e.Fields, model.Field{Key: g.str(8), Value: g.secret(g.str(25)), Protected: g.bool()})
	}
	if len(g.pool) > 0 {
		for i := g.r.IntN(3); i > 0; i-- {
			e.Attachments = append(e.Attachments, model.Attachment{Name: g.str(10), Binary: g.pool[g.r.IntN(len(g.pool))].key})
		}
	}
	e.AutoType = model.AutoType{Enabled: g.bool(), DataTransferObfuscation: g.r.IntN(2), DefaultSequence: g.str(15)}
	for i := g.r.IntN(3); i > 0; i-- {
		e.AutoType.Associations = append(e.AutoType.Associations, model.AutoTypeAssociation{Window: g.str(15), Sequence: g.str(15)})
	}
	e.CustomData = g.customData(g.o.WithItemCustomData)
	if g.o.With41 {
		if g.bool() {
			q := g.bool()
			e.QualityCheck = &q
		}
		e.PreviousParent = g.maybeUUID()
	}
	if g.o.WithRaw && g.r.IntN(4) == 0 {
		e.Raw = model.Raw{Attrs: g.attrs(), Nodes: g.rawNodes(2)}
	}
	if withHistory {
		for i := g.r.IntN(4); i > 0; i-- {
			h := g.entry(false)
			h.UUID = e.UUID
			e.History = append(e.History, h)
		}
	}
	return e
}

func (g *gen) attrs() []model.RawAttr {
	var attrs []model.RawAttr
	for i := g.r.IntN(3); i > 0; i-- {
		attrs = append(attrs, model.RawAttr{Name: "x-" + g.word() + string(rune('a'+i)), Value: g.str(10)})
	}
	return attrs
}

// rawNodes makes unknown elements, nested up to depth levels. A node with
// children has no text of its own that is only white space, because a reader takes
// that for layout.
func (g *gen) rawNodes(depth int) []*model.RawNode {
	var nodes []*model.RawNode
	for i := 1 + g.r.IntN(2); i > 0; i-- {
		n := &model.RawNode{Name: "X-" + g.word(), Attrs: g.attrs()}
		if depth > 1 && g.bool() {
			n.Children = g.rawNodes(depth - 1)
			if g.bool() {
				n.Text = g.secret("t" + g.word())
			}
		} else {
			n.Text = g.secret(g.str(20))
		}
		nodes = append(nodes, n)
	}
	return nodes
}
