// Command docgen generates the chain's reference documentation.
//
// Reference docs that are written by hand drift, and a reference that is subtly
// wrong is worse than none — somebody trusts it, ships against it, and finds out
// in production. So everything here is derived from the things the chain is
// actually built from:
//
//   - message and query shapes, field names, and every doc comment come from the
//     protobuf descriptors, via buf;
//   - who may sign a message comes from its `cosmos.msg.v1.signer` option, the
//     same option the SDK enforces at runtime;
//   - REST routes come from the `google.api.http` annotations that define them;
//   - error codes and text come from each module's registered sentinel errors;
//   - parameter defaults come from calling the module's own DefaultParams().
//
// The consequence is that a change to the chain either shows up in these docs on
// the next `make docs`, or it is not a change to anything a client can observe.
// `make docs-check` fails the build if the committed output has fallen behind.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"text/template"

	msgv1 "cosmossdk.io/api/cosmos/msg/v1"
	"google.golang.org/genproto/googleapis/api/annotations"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	ammtypes "yamale/blockchain/x/amm/types"
	builderfeetypes "yamale/blockchain/x/builderfee/types"
	emissiontypes "yamale/blockchain/x/emission/types"
	nettingtypes "yamale/blockchain/x/netting/types"
	paymsgtypes "yamale/blockchain/x/paymsg/types"
	stablecointypes "yamale/blockchain/x/stablecoin/types"
	treasurytypes "yamale/blockchain/x/treasury/types"
	validatorgovtypes "yamale/blockchain/x/validatorgov/types"
)

// moduleBlurb is the one-sentence answer to "what is this module for?", which
// no amount of descriptor walking can produce. Everything else on the page is
// generated.
var moduleBlurb = map[string]string{
	"amm":          "A constant-product automated market maker: permissionless liquidity pools, and swaps priced by the pool's own reserves.",
	"builderfee":   "Shares a governance-set portion of transaction fees with the developer whose message type was used.",
	"emission":     "Replaces the standard mint module with a fixed, decaying issuance schedule that converges on a capped supply.",
	"netting":      "The tiered settlement layer: participants settle retail activity on their own books and submit only what they owe each other, netted multilaterally against prefunded reserves, with high-value items settling gross.",
	"paymsg":       "ISO 20022-shaped credit transfers between institutions that governance has approved, each leaving a queryable statement entry.",
	"stablecoin":   "Governance-approved issuers for fiat-referenced currencies, with minting and redemption restricted to the approved issuer of each denom.",
	"treasury":     "Programmable custody: shared funds with roles, spending policies, time locks and vesting schedules, where committed funds cannot be spent by anyone.",
	"validatorgov": "Restricts the validator set to candidates that governance has admitted, enforced before a create-validator transaction is accepted.",
}

// defaultParams returns each module's own defaults, so the documented values are
// the ones the chain would actually start with.
var defaultParams = map[string]any{
	"amm":          ammtypes.DefaultParams(),
	"builderfee":   builderfeetypes.DefaultParams(),
	"emission":     emissiontypes.DefaultParams(),
	"netting":      nettingtypes.DefaultParams(),
	"paymsg":       paymsgtypes.DefaultParams(),
	"stablecoin":   stablecointypes.DefaultParams(),
	"treasury":     treasurytypes.DefaultParams(),
	"validatorgov": validatorgovtypes.DefaultParams(),
}

func main() {
	var (
		descriptorPath = flag.String("descriptor", "", "FileDescriptorSet produced by buf build --include-source-info")
		outDir         = flag.String("out", "docs/reference", "directory to write the reference into")
		repoRoot       = flag.String("root", ".", "repository root, for reading error registries")
	)
	flag.Parse()

	if *descriptorPath == "" {
		fail("a --descriptor is required")
	}

	raw, err := os.ReadFile(*descriptorPath)
	if err != nil {
		fail("reading descriptor: %v", err)
	}

	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(raw, &set); err != nil {
		fail("parsing descriptor: %v", err)
	}

	modules := collectModules(&set)
	if len(modules) == 0 {
		fail("no blockchain.* protobuf packages found in the descriptor")
	}

	for name, mod := range modules {
		mod.Blurb = moduleBlurb[name]
		mod.Errors = readErrors(filepath.Join(*repoRoot, "x", name, "types", "errors.go"))
		mod.Params = readParams(name, mod.ParamsFields)
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fail("creating %s: %v", *outDir, err)
	}

	names := make([]string, 0, len(modules))
	for name := range modules {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		path := filepath.Join(*outDir, name+".md")
		content, err := render(modules[name])
		if err != nil {
			fail("rendering %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			fail("writing %s: %v", path, err)
		}
		fmt.Printf("wrote %s (%d messages, %d queries, %d errors)\n",
			path, len(modules[name].Messages), len(modules[name].Queries), len(modules[name].Errors))
	}

	index, err := renderIndex(names, modules)
	if err != nil {
		fail("rendering index: %v", err)
	}
	indexPath := filepath.Join(*outDir, "README.md")
	if err := os.WriteFile(indexPath, []byte(index), 0o644); err != nil {
		fail("writing %s: %v", indexPath, err)
	}
	fmt.Printf("wrote %s\n", indexPath)
}

// ---------------------------------------------------------------- model

type Module struct {
	Name          string
	Blurb         string
	Messages      []Message
	Queries       []Query
	States        []State
	Enums         []Enum
	Params        []Param
	Errors        []ErrorDoc
	ParamsMessage *descriptorpb.DescriptorProto
	// ParamsFields carries the Params message's field comments, so each
	// documented default keeps the explanation written beside it in the .proto.
	ParamsFields []Field
}

type Message struct {
	Name        string
	TypeURL     string
	Signer      string
	Doc         string
	Fields      []Field
	ResponseDoc string
}

type Query struct {
	Name     string
	Doc      string
	Route    string
	Request  []Field
	Response []Field
}

type State struct {
	Name   string
	Doc    string
	Fields []Field
}

// Enum documents a closed set of values a field may take. Without it a reader
// sees a field typed `Role` with no way to learn what may go in it.
type Enum struct {
	Name   string
	Doc    string
	Values []EnumValue
}

type EnumValue struct {
	Name string
	Doc  string
}

type Field struct {
	Name     string
	Type     string
	Repeated bool
	Doc      string
}

type Param struct {
	Name    string
	Default string
	Doc     string
}

type ErrorDoc struct {
	Name string
	Code string
	Text string
}

// ---------------------------------------------------------------- descriptors

// collectModules walks the descriptor set and groups everything by module.
func collectModules(set *descriptorpb.FileDescriptorSet) map[string]*Module {
	modules := map[string]*Module{}

	// Message shapes are needed across files within a module, so index them all
	// before interpreting any service.
	byFullName := map[string]*descriptorpb.DescriptorProto{}
	enumsByModule := map[string][]Enum{}
	docsByFullName := map[string]string{}
	fieldsByFullName := map[string][]Field{}

	type located struct {
		file *descriptorpb.FileDescriptorProto
		idx  int
	}
	locations := map[string]located{}

	for _, file := range set.GetFile() {
		pkg := file.GetPackage()
		if !strings.HasPrefix(pkg, "blockchain.") {
			continue
		}
		comments := commentIndex(file)
		for i, enum := range file.GetEnumType() {
			values := make([]EnumValue, 0, len(enum.GetValue()))
			for vi, v := range enum.GetValue() {
				values = append(values, EnumValue{Name: v.GetName(), Doc: comments[pathKey(5, i, 2, vi)]})
			}
			enumsByModule[moduleName(pkg)] = append(enumsByModule[moduleName(pkg)], Enum{
				Name:   enum.GetName(),
				Doc:    comments[pathKey(5, i)],
				Values: values,
			})
		}
		for i, msg := range file.GetMessageType() {
			full := pkg + "." + msg.GetName()
			byFullName[full] = msg
			docsByFullName[full] = comments[pathKey(4, i)]
			fieldsByFullName[full] = fieldsOf(msg, comments, i)
			locations[full] = located{file: file, idx: i}
		}
	}

	usedAsRPCType := map[string]bool{}

	for _, file := range set.GetFile() {
		pkg := file.GetPackage()
		name := moduleName(pkg)
		if name == "" {
			continue
		}
		mod := modules[name]
		if mod == nil {
			mod = &Module{Name: name}
			modules[name] = mod
		}

		comments := commentIndex(file)

		for si, svc := range file.GetService() {
			for mi, method := range svc.GetMethod() {
				doc := comments[pathKey(6, si, 2, mi)]
				in := strings.TrimPrefix(method.GetInputType(), ".")
				out := strings.TrimPrefix(method.GetOutputType(), ".")
				usedAsRPCType[in] = true
				usedAsRPCType[out] = true

				switch svc.GetName() {
				case "Msg":
					mod.Messages = append(mod.Messages, Message{
						Name:    method.GetInputType()[strings.LastIndex(method.GetInputType(), ".")+1:],
						TypeURL: "/" + in,
						Signer:  signerOf(byFullName[in]),
						Doc:     firstNonEmpty(doc, docsByFullName[in]),
						Fields:  fieldsByFullName[in],
					})
				case "Query":
					mod.Queries = append(mod.Queries, Query{
						Name:     method.GetName(),
						Doc:      doc,
						Route:    httpRouteOf(method),
						Request:  fieldsByFullName[in],
						Response: fieldsByFullName[out],
					})
				}
			}
		}
	}

	// Anything not used as an RPC request or response is state or a nested value
	// type — the things a reader needs when interpreting a query result.
	for full, msg := range byFullName {
		name := moduleName(packageOf(full))
		if name == "" || modules[name] == nil {
			continue
		}
		if msg.GetName() == "Params" {
			modules[name].ParamsMessage = msg
			modules[name].ParamsFields = fieldsByFullName[full]
			continue
		}
		if usedAsRPCType[full] || strings.HasPrefix(msg.GetName(), "Msg") || strings.HasPrefix(msg.GetName(), "Query") {
			continue
		}
		if msg.GetName() == "GenesisState" || msg.GetName() == "Module" {
			continue
		}
		modules[name].States = append(modules[name].States, State{
			Name:   msg.GetName(),
			Doc:    docsByFullName[full],
			Fields: fieldsByFullName[full],
		})
	}

	for name, enums := range enumsByModule {
		if name == "" || modules[name] == nil {
			continue
		}
		modules[name].Enums = append(modules[name].Enums, enums...)
	}

	for _, mod := range modules {
		sort.Slice(mod.Enums, func(i, j int) bool { return mod.Enums[i].Name < mod.Enums[j].Name })
		sort.Slice(mod.Messages, func(i, j int) bool { return mod.Messages[i].Name < mod.Messages[j].Name })
		sort.Slice(mod.Queries, func(i, j int) bool { return mod.Queries[i].Name < mod.Queries[j].Name })
		sort.Slice(mod.States, func(i, j int) bool { return mod.States[i].Name < mod.States[j].Name })
	}

	return modules
}

// signerOf reads the cosmos.msg.v1.signer option — the same declaration the SDK
// uses to decide whose signature a message requires.
func signerOf(msg *descriptorpb.DescriptorProto) string {
	if msg == nil || msg.Options == nil {
		return ""
	}
	value := proto.GetExtension(msg.Options, msgv1.E_Signer)
	signers, ok := value.([]string)
	if !ok || len(signers) == 0 {
		return ""
	}
	return strings.Join(signers, ", ")
}

// httpRouteOf reads the google.api.http annotation that defines a query's REST
// route.
func httpRouteOf(method *descriptorpb.MethodDescriptorProto) string {
	if method.Options == nil {
		return ""
	}
	value := proto.GetExtension(method.Options, annotations.E_Http)
	rule, ok := value.(*annotations.HttpRule)
	if !ok || rule == nil {
		return ""
	}
	switch pattern := rule.GetPattern().(type) {
	case *annotations.HttpRule_Get:
		return "GET " + pattern.Get
	case *annotations.HttpRule_Post:
		return "POST " + pattern.Post
	default:
		return ""
	}
}

func fieldsOf(msg *descriptorpb.DescriptorProto, comments map[string]string, msgIndex int) []Field {
	fields := make([]Field, 0, len(msg.GetField()))
	for fi, f := range msg.GetField() {
		fields = append(fields, Field{
			Name:     f.GetName(),
			Type:     typeName(f),
			Repeated: f.GetLabel() == descriptorpb.FieldDescriptorProto_LABEL_REPEATED,
			Doc:      comments[pathKey(4, msgIndex, 2, fi)],
		})
	}
	return fields
}

func typeName(f *descriptorpb.FieldDescriptorProto) string {
	switch f.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		full := strings.TrimPrefix(f.GetTypeName(), ".")
		return full[strings.LastIndex(full, ".")+1:]
	default:
		return strings.ToLower(strings.TrimPrefix(f.GetType().String(), "TYPE_"))
	}
}

// commentIndex maps a descriptor path to its leading comment, which is where
// every explanation written in the .proto files ends up.
func commentIndex(file *descriptorpb.FileDescriptorProto) map[string]string {
	out := map[string]string{}
	for _, loc := range file.GetSourceCodeInfo().GetLocation() {
		text := strings.TrimSpace(loc.GetLeadingComments())
		if text == "" {
			continue
		}
		parts := make([]any, 0, len(loc.GetPath()))
		for _, p := range loc.GetPath() {
			parts = append(parts, int(p))
		}
		out[pathKey(parts...)] = cleanComment(text)
	}
	return out
}

func pathKey(parts ...any) string {
	segments := make([]string, 0, len(parts))
	for _, p := range parts {
		segments = append(segments, fmt.Sprint(p))
	}
	return strings.Join(segments, ".")
}

// cleanComment turns a proto comment block into a paragraph, preserving blank
// lines so multi-paragraph explanations survive.
func cleanComment(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(strings.TrimSpace(line), "* ")
	}

	var paragraphs []string
	var current []string
	for _, line := range lines {
		if line == "" {
			if len(current) > 0 {
				paragraphs = append(paragraphs, strings.Join(current, " "))
				current = nil
			}
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		paragraphs = append(paragraphs, strings.Join(current, " "))
	}
	return strings.Join(paragraphs, "\n\n")
}

func moduleName(pkg string) string {
	// blockchain.<module>.v1, but not blockchain.<module>.module.v1
	parts := strings.Split(pkg, ".")
	if len(parts) != 3 || parts[0] != "blockchain" {
		return ""
	}
	return parts[1]
}

func packageOf(fullName string) string {
	return fullName[:strings.LastIndex(fullName, ".")]
}

// ---------------------------------------------------------------- Go sources

// readErrors extracts a module's registered sentinel errors. These are the exact
// codes a client will see in a failed transaction, so they belong in the
// reference even though they live in Go rather than in a .proto.
func readErrors(path string) []ErrorDoc {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil
	}

	var out []ErrorDoc
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) == 0 || len(spec.Values) == 0 {
			return true
		}
		call, ok := spec.Values[0].(*ast.CallExpr)
		if !ok || len(call.Args) < 3 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Register" {
			return true
		}

		code := literal(call.Args[1])
		text := literal(call.Args[2])
		if code == "" {
			return true
		}
		out = append(out, ErrorDoc{Name: spec.Names[0].Name, Code: code, Text: text})
		return true
	})

	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func literal(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok {
		return ""
	}
	if lit.Kind == token.STRING {
		unquoted, err := strconv.Unquote(lit.Value)
		if err != nil {
			return lit.Value
		}
		return unquoted
	}
	return lit.Value
}

// readParams pairs each parameter's real default with the explanation written
// beside it in the .proto.
//
// Matching is by proto field name rather than by position, so reordering either
// the struct or the message cannot silently attach a default to the wrong
// explanation.
func readParams(module string, fields []Field) []Param {
	defaults, ok := defaultParams[module]
	if !ok || len(fields) == 0 {
		return nil
	}

	docs := make(map[string]string, len(fields))
	for _, f := range fields {
		docs[f.Name] = f.Doc
	}

	value := reflect.ValueOf(defaults)
	typ := value.Type()

	var out []Param
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		name := protoNameOf(field)
		doc, known := docs[name]
		if !known {
			continue
		}
		out = append(out, Param{
			Name:    name,
			Default: fmt.Sprintf("%v", value.Field(i).Interface()),
			Doc:     doc,
		})
	}
	return out
}

// protoNameOf recovers a field's proto name from its struct tag, since that is
// what a client sends and reads.
func protoNameOf(field reflect.StructField) string {
	tag := field.Tag.Get("protobuf")
	for _, part := range strings.Split(tag, ",") {
		if strings.HasPrefix(part, "name=") {
			return strings.TrimPrefix(part, "name=")
		}
	}
	return strings.ToLower(field.Name)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "docgen: "+format+"\n", args...)
	os.Exit(1)
}

// ---------------------------------------------------------------- rendering

const moduleTemplate = `<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen from the protobuf descriptors, the module's registered
errors, and its DefaultParams(). Run ` + "`make docs`" + ` to regenerate.
-->

# x/{{ .Name }}

{{ with .Blurb }}{{ . }}
{{ end }}
{{ if .Messages }}
## Transactions

{{ range .Messages }}
### {{ .Name }}

` + "`{{ .TypeURL }}`" + `
{{ with .Signer }}
Signed by the ` + "`{{ . }}`" + ` field.
{{ end }}{{ with .Doc }}
{{ . }}
{{ end }}{{ if .Fields }}
| Field | Type | Description |
| --- | --- | --- |
{{ range .Fields }}| ` + "`{{ .Name }}`" + ` | {{ if .Repeated }}repeated {{ end }}{{ .Type }} | {{ .Doc | oneline }} |
{{ end }}{{ end }}
{{ end }}{{ end }}
{{ if .Queries }}
## Queries

{{ range .Queries }}
### {{ .Name }}
{{ with .Route }}
` + "`{{ . }}`" + `
{{ end }}{{ with .Doc }}
{{ . }}
{{ end }}{{ if .Request }}
Request:

| Field | Type | Description |
| --- | --- | --- |
{{ range .Request }}| ` + "`{{ .Name }}`" + ` | {{ if .Repeated }}repeated {{ end }}{{ .Type }} | {{ .Doc | oneline }} |
{{ end }}{{ end }}{{ if .Response }}
Response:

| Field | Type | Description |
| --- | --- | --- |
{{ range .Response }}| ` + "`{{ .Name }}`" + ` | {{ if .Repeated }}repeated {{ end }}{{ .Type }} | {{ .Doc | oneline }} |
{{ end }}{{ end }}
{{ end }}{{ end }}
{{ if .States }}
## State

{{ range .States }}
### {{ .Name }}
{{ with .Doc }}
{{ . }}
{{ end }}{{ if .Fields }}
| Field | Type | Description |
| --- | --- | --- |
{{ range .Fields }}| ` + "`{{ .Name }}`" + ` | {{ if .Repeated }}repeated {{ end }}{{ .Type }} | {{ .Doc | oneline }} |
{{ end }}{{ end }}
{{ end }}{{ end }}
{{ if .Enums }}
## Value types

{{ range .Enums }}
### {{ .Name }}
{{ with .Doc }}
{{ . }}
{{ end }}
| Value | Meaning |
| --- | --- |
{{ range .Values }}| ` + "`{{ .Name }}`" + ` | {{ .Doc | oneline }} |
{{ end }}
{{ end }}{{ end }}
{{ if .Params }}
## Parameters

Changed by governance through ` + "`MsgUpdateParams`" + `. Defaults are the values a chain starts with at genesis.

| Parameter | Default | Description |
| --- | --- | --- |
{{ range .Params }}| ` + "`{{ .Name }}`" + ` | ` + "`{{ .Default }}`" + ` | {{ .Doc | oneline }} |
{{ end }}{{ end }}
{{ if .Errors }}
## Errors

Every way a transaction to this module can be rejected.

| Code | Name | Message |
| --- | --- | --- |
{{ range .Errors }}| {{ .Code }} | ` + "`{{ .Name }}`" + ` | {{ .Text }} |
{{ end }}{{ end }}`

const indexTemplate = `<!--
GENERATED FILE — DO NOT EDIT.
Produced by tools/docgen. Run ` + "`make docs`" + ` to regenerate.
-->

# Reference

Every message, query, state type, parameter and error code on the chain,
generated from the protobuf definitions and the modules' own source. If
something here is wrong, the code is wrong — these pages cannot drift from it.

For explanations and walkthroughs, start with the [guides](../guides/).

| Module | Purpose | Transactions | Queries |
| --- | --- | --- | --- |
{{ range .Modules }}| [x/{{ .Name }}]({{ .Name }}.md) | {{ .Blurb | oneline }} | {{ len .Messages }} | {{ len .Queries }} |
{{ end }}
## Chain-wide conventions

**Amounts** are integers in the base unit, with no decimal point. ` + "`uyml`" + ` is the
base unit of YML at six decimal places, so ` + "`12500000uyml`" + ` is 12.5 YML. Clients
convert only when displaying.

**Addresses** are bech32 with the ` + "`yml`" + ` prefix; validator operator addresses use
` + "`ymlvaloper`" + `.

**Signers** are declared per message and enforced by the SDK. A message whose
signer is the governance module account cannot be sent directly — it has to be
the payload of a governance proposal.

**Errors** carry a module codespace and the code listed on each page, so a
failed transaction can be traced to exactly one registered error.
`

func render(mod *Module) (string, error) {
	tmpl, err := template.New("module").Funcs(templateFuncs()).Parse(moduleTemplate)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, mod); err != nil {
		return "", err
	}
	return tidy(out.String()), nil
}

func renderIndex(names []string, modules map[string]*Module) (string, error) {
	tmpl, err := template.New("index").Funcs(templateFuncs()).Parse(indexTemplate)
	if err != nil {
		return "", err
	}
	ordered := make([]*Module, 0, len(names))
	for _, n := range names {
		ordered = append(ordered, modules[n])
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, struct{ Modules []*Module }{ordered}); err != nil {
		return "", err
	}
	return tidy(out.String()), nil
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		// oneline flattens a comment for a table cell, where a newline would
		// break the row.
		"oneline": func(s string) string {
			s = strings.ReplaceAll(s, "\n", " ")
			s = strings.ReplaceAll(s, "|", "\\|")
			return strings.Join(strings.Fields(s), " ")
		},
	}
}

// tidy collapses the blank lines that fall out of template control flow.
func tidy(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s) + "\n"
}
