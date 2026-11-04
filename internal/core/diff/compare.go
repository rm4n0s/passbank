package diff

import (
	"slices"
	"sort"
	"strings"

	"github.com/rm4n0s/passbank/internal/core/model"
)

// The names of the fields a difference can be in, as the change list and the
// plan file show them. They never contain values.
const (
	FieldAttachments = "attachments"
	FieldIcon        = "icon"
	FieldColors      = "colors"
	FieldOverrideURL = "override-url"
	FieldTags        = "tags"
	FieldExpiry      = "expiry"
	FieldAutoType    = "auto-type"
	FieldCustomData  = "custom-data"
	FieldExtraXML    = "extra-xml"
	FieldGroup       = "group"
	FieldParent      = "parent"
	FieldName        = "name"
	FieldNotes       = "notes"
	FieldGroupConfig = "settings"

	FieldDescription      = "description"
	FieldDefaultUserName  = "default-username"
	FieldColor            = "color"
	FieldHistoryLimits    = "history-limits"
	FieldRecycleBin       = "recycle-bin"
	FieldMemoryProtection = "memory-protection"
)

// standardFields maps the standard string fields to the names shown.
var standardFields = map[string]string{
	model.FieldTitle:    "title",
	model.FieldUserName: "username",
	model.FieldPassword: "password",
	model.FieldURL:      "url",
	model.FieldNotes:    "notes",
}

// fieldLabel names a string field: the standard ones by a short lower-case name,
// the others as `field <key>`.
func fieldLabel(key string) string {
	if n, ok := standardFields[key]; ok {
		return n
	}
	return "field " + key
}

// EntryDifferences lists the names of the fields in which two entries differ,
// as SPEC §8 defines equality: string fields (values and protected flags),
// attachments (name and content hash), icon, colors, override URL, tags, expiry,
// auto-type settings, custom data and extra XML. Last access, usage count and
// history are not compared. The parent group is compared by the caller, which
// knows how the groups of the two databases pair. The names are in a fixed order
// and never contain values.
func EntryDifferences(a, b *model.Entry) []string {
	var out []string
	out = append(out, fieldDifferences(a.Fields, b.Fields)...)
	if !slices.Equal(attachmentSet(a.Attachments), attachmentSet(b.Attachments)) {
		out = append(out, FieldAttachments)
	}
	if a.IconID != b.IconID || a.CustomIcon != b.CustomIcon {
		out = append(out, FieldIcon)
	}
	if a.ForegroundColor != b.ForegroundColor || a.BackgroundColor != b.BackgroundColor {
		out = append(out, FieldColors)
	}
	if a.OverrideURL != b.OverrideURL {
		out = append(out, FieldOverrideURL)
	}
	if !sameTags(a.Tags, b.Tags) {
		out = append(out, FieldTags)
	}
	if expiryDiffers(a.Times, b.Times) {
		out = append(out, FieldExpiry)
	}
	if !autoTypeEqual(a.AutoType, b.AutoType) {
		out = append(out, FieldAutoType)
	}
	if !customDataEqual(a.CustomData, b.CustomData) {
		out = append(out, FieldCustomData)
	}
	if !rawEqual(a.Raw, b.Raw) {
		out = append(out, FieldExtraXML)
	}
	return out
}

// fieldDifferences names the string fields whose value or protected flag differs,
// or that only one entry has, in the order they appear in a and then b.
func fieldDifferences(a, b []model.Field) []string {
	var out []string
	seen := map[string]bool{}
	check := func(f model.Field, other []model.Field) {
		if seen[f.Key] {
			return
		}
		seen[f.Key] = true
		i := slices.IndexFunc(other, func(o model.Field) bool { return o.Key == f.Key })
		if i < 0 || other[i].Protected != f.Protected || !other[i].Value.Equal(f.Value) {
			out = append(out, fieldLabel(f.Key))
		}
	}
	for _, f := range a {
		check(f, b)
	}
	for _, f := range b {
		check(f, a)
	}
	return out
}

// attachmentSet is the attachments as sorted "name hash" strings, so that their
// order and the protected flag of the pool do not count.
func attachmentSet(as []model.Attachment) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.Name + "\x00" + string(a.Binary.Hash[:])
	}
	sort.Strings(out)
	return out
}

func sameTags(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	sort.Strings(x)
	sort.Strings(y)
	return slices.Equal(x, y)
}

func expiryDiffers(a, b model.Times) bool {
	if a.Expires != b.Expires {
		return true
	}
	return a.Expires && !a.Expiry.Equal(b.Expiry)
}

func autoTypeEqual(a, b model.AutoType) bool {
	return a.Enabled == b.Enabled && a.DataTransferObfuscation == b.DataTransferObfuscation &&
		a.DefaultSequence == b.DefaultSequence && slices.Equal(a.Associations, b.Associations)
}

// customDataEqual compares custom data by key and value; the order and the
// modification times do not count.
func customDataEqual(a, b []model.CustomDataItem) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]string, len(a))
	for _, it := range a {
		m[it.Key] = it.Value
	}
	for _, it := range b {
		if v, ok := m[it.Key]; !ok || v != it.Value {
			return false
		}
	}
	return true
}

func rawEqual(a, b model.Raw) bool {
	if !slices.Equal(a.Attrs, b.Attrs) || len(a.Nodes) != len(b.Nodes) {
		return false
	}
	for i := range a.Nodes {
		if !rawNodeEqual(a.Nodes[i], b.Nodes[i]) {
			return false
		}
	}
	return true
}

func rawNodeEqual(a, b *model.RawNode) bool {
	if a.Name != b.Name || !slices.Equal(a.Attrs, b.Attrs) || !a.Text.Equal(b.Text) || len(a.Children) != len(b.Children) {
		return false
	}
	for i := range a.Children {
		if !rawNodeEqual(a.Children[i], b.Children[i]) {
			return false
		}
	}
	return true
}

// GroupDifferences lists the names of the properties in which two paired groups
// differ: name, notes, icon, tags, expiry, settings (default auto-type sequence,
// auto-type and searching), custom data and extra XML. The parent is compared by
// the caller. Whether the group is expanded and which entry was on top are
// interface state and do not count.
func GroupDifferences(a, b *model.Group) []string {
	var out []string
	if a.Name != b.Name {
		out = append(out, FieldName)
	}
	if a.Notes != b.Notes {
		out = append(out, FieldNotes)
	}
	if a.IconID != b.IconID || a.CustomIcon != b.CustomIcon {
		out = append(out, FieldIcon)
	}
	if !sameTags(a.Tags, b.Tags) {
		out = append(out, FieldTags)
	}
	if expiryDiffers(a.Times, b.Times) {
		out = append(out, FieldExpiry)
	}
	if a.DefaultAutoTypeSequence != b.DefaultAutoTypeSequence || a.EnableAutoType != b.EnableAutoType || a.EnableSearching != b.EnableSearching {
		out = append(out, FieldGroupConfig)
	}
	if !customDataEqual(a.CustomData, b.CustomData) {
		out = append(out, FieldCustomData)
	}
	if !rawEqual(a.Raw, b.Raw) {
		out = append(out, FieldExtraXML)
	}
	return out
}

// SettingsDifferences lists the names of the Meta settings in which two
// databases differ: name, description, default user name, color, history
// limits, recycle-bin on/off and memory protection (SPEC §8).
func SettingsDifferences(a, b *model.Meta) []string {
	var out []string
	if a.DatabaseName != b.DatabaseName {
		out = append(out, FieldName)
	}
	if a.Description != b.Description {
		out = append(out, FieldDescription)
	}
	if a.DefaultUserName != b.DefaultUserName {
		out = append(out, FieldDefaultUserName)
	}
	if a.Color != b.Color {
		out = append(out, FieldColor)
	}
	if a.HistoryMaxItems != b.HistoryMaxItems || a.HistoryMaxSize != b.HistoryMaxSize || a.MaintenanceHistoryDays != b.MaintenanceHistoryDays {
		out = append(out, FieldHistoryLimits)
	}
	if a.RecycleBinEnabled != b.RecycleBinEnabled {
		out = append(out, FieldRecycleBin)
	}
	if a.MemoryProtection != b.MemoryProtection {
		out = append(out, FieldMemoryProtection)
	}
	return out
}

// joinNames joins field names for display.
func joinNames(names []string) string { return strings.Join(names, ", ") }
