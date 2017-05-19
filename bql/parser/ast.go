package parser

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/sensorbee/sensorbee.v0/data"
)

// Expression represents operator of each syntax tree.
type Expression interface {
	ReferencedRelations() map[string]bool
	RenameReferencedRelation(string, string) Expression
	Foldable() bool
	String() string
}

// This file holds a set of structs that make up the Abstract
// Syntax Tree of a BQL statement. Usually, for every rule in
// the PEG file, the left side should correspond to a struct
// in this file with the same name.

// Combined Structures (all with *AST)

// SelectStmt represents SELECT syntax.
type SelectStmt struct {
	EmitterAST
	ProjectionsAST
	WindowedFromAST
	FilterAST
	GroupingAST
	HavingAST
}

func (s SelectStmt) String() string {
	str := []string{"SELECT", s.EmitterAST.string()}
	str = append(str, s.ProjectionsAST.string())
	str = append(str, s.WindowedFromAST.string())
	str = append(str, s.FilterAST.string())
	str = append(str, s.GroupingAST.string())
	str = append(str, s.HavingAST.string())

	st := []string{}
	for _, s := range str {
		if s != "" {
			st = append(st, s)
		}
	}
	return strings.Join(st, " ")
}

// SelectUnionStmt represents SELECT .. UNION syntax.
type SelectUnionStmt struct {
	Selects []SelectStmt
}

func (s SelectUnionStmt) String() string {
	str := make([]string, len(s.Selects))
	for i, s := range s.Selects {
		str[i] = s.String()
	}
	return strings.Join(str, " UNION ALL ")
}

// CreateStreamAsSelectStmt represents CREATE STREAM .. AS SELECT syntax.
type CreateStreamAsSelectStmt struct {
	Name   StreamIdentifier
	Select SelectStmt
}

func (s CreateStreamAsSelectStmt) String() string {
	str := []string{"CREATE", "STREAM", string(s.Name), "AS", s.Select.String()}
	return strings.Join(str, " ")
}

// CreateStreamAsSelectUnionStmt represents CREATE STREAM .. AS SELECT .. UNION
// syntax.
type CreateStreamAsSelectUnionStmt struct {
	Name StreamIdentifier
	SelectUnionStmt
}

func (s CreateStreamAsSelectUnionStmt) String() string {
	str := []string{"CREATE", "STREAM", string(s.Name), "AS", s.SelectUnionStmt.String()}
	return strings.Join(str, " ")
}

// CreateSourceStmt represents CREATE SOURCE .. syntax.
type CreateSourceStmt struct {
	Paused BinaryKeyword
	Name   StreamIdentifier
	Type   SourceSinkType
	SourceSinkSpecsAST
}

func (s CreateSourceStmt) String() string {
	str := []string{"CREATE", "SOURCE", string(s.Name), "TYPE", string(s.Type)}
	paused := s.Paused.string("PAUSED", "UNPAUSED")
	if paused != "" {
		str = append(str[:1], append([]string{paused}, str[1:]...)...)
	}
	specs := s.SourceSinkSpecsAST.string("WITH")
	if specs != "" {
		str = append(str, specs)
	}
	return strings.Join(str, " ")
}

// CreateSinkStmt represents CREATE SINK .. syntax.
type CreateSinkStmt struct {
	Name StreamIdentifier
	Type SourceSinkType
	SourceSinkSpecsAST
}

func (s CreateSinkStmt) String() string {
	str := []string{"CREATE", "SINK", string(s.Name), "TYPE", string(s.Type)}
	specs := s.SourceSinkSpecsAST.string("WITH")
	if specs != "" {
		str = append(str, specs)
	}
	return strings.Join(str, " ")
}

// CreateStateStmt represents CREATE STATE .. syntax.
type CreateStateStmt struct {
	Name StreamIdentifier
	Type SourceSinkType
	SourceSinkSpecsAST
}

func (s CreateStateStmt) String() string {
	str := []string{"CREATE", "STATE", string(s.Name), "TYPE", string(s.Type)}
	specs := s.SourceSinkSpecsAST.string("WITH")
	if specs != "" {
		str = append(str, specs)
	}
	return strings.Join(str, " ")
}

// UpdateStateStmt represents UPDATE STATE .. syntax.
type UpdateStateStmt struct {
	Name StreamIdentifier
	SourceSinkSpecsAST
}

func (s UpdateStateStmt) String() string {
	str := []string{"UPDATE", "STATE", string(s.Name)}
	specs := s.SourceSinkSpecsAST.string("SET")
	if specs != "" {
		str = append(str, specs)
	}
	return strings.Join(str, " ")
}

// UpdateSourceStmt represents UPDATE SOURCE .. syntax.
type UpdateSourceStmt struct {
	Name StreamIdentifier
	SourceSinkSpecsAST
}

func (s UpdateSourceStmt) String() string {
	str := []string{"UPDATE", "SOURCE", string(s.Name)}
	specs := s.SourceSinkSpecsAST.string("SET")
	if specs != "" {
		str = append(str, specs)
	}
	return strings.Join(str, " ")
}

// UpdateSinkStmt represents UPDATE SINK .. syntax.
type UpdateSinkStmt struct {
	Name StreamIdentifier
	SourceSinkSpecsAST
}

func (s UpdateSinkStmt) String() string {
	str := []string{"UPDATE", "SINK", string(s.Name)}
	specs := s.SourceSinkSpecsAST.string("SET")
	if specs != "" {
		str = append(str, specs)
	}
	return strings.Join(str, " ")
}

// InsertIntoFromStmt represents INSERT INTO .. FROM syntax.
type InsertIntoFromStmt struct {
	Sink  StreamIdentifier
	Input StreamIdentifier
}

func (s InsertIntoFromStmt) String() string {
	str := []string{"INSERT", "INTO", string(s.Sink), "FROM", string(s.Input)}
	return strings.Join(str, " ")
}

// PauseSourceStmt represents PAUSE SOURCE .. syntax.
type PauseSourceStmt struct {
	Source StreamIdentifier
}

func (s PauseSourceStmt) String() string {
	str := []string{"PAUSE", "SOURCE", string(s.Source)}
	return strings.Join(str, " ")
}

// ResumeSourceStmt represents RESUME SOURCE .. syntax.
type ResumeSourceStmt struct {
	Source StreamIdentifier
}

func (s ResumeSourceStmt) String() string {
	str := []string{"RESUME", "SOURCE", string(s.Source)}
	return strings.Join(str, " ")
}

// RewindSourceStmt represents REWIND SOURCE .. syntax.
type RewindSourceStmt struct {
	Source StreamIdentifier
}

func (s RewindSourceStmt) String() string {
	str := []string{"REWIND", "SOURCE", string(s.Source)}
	return strings.Join(str, " ")
}

// DropSourceStmt represents DROP SOURCE .. syntax.
type DropSourceStmt struct {
	Source StreamIdentifier
}

func (s DropSourceStmt) String() string {
	str := []string{"DROP", "SOURCE", string(s.Source)}
	return strings.Join(str, " ")
}

// DropStreamStmt represents DROP STREAM .. syntax.
type DropStreamStmt struct {
	Stream StreamIdentifier
}

func (s DropStreamStmt) String() string {
	str := []string{"DROP", "STREAM", string(s.Stream)}
	return strings.Join(str, " ")
}

// DropSinkStmt represents DROP SINK .. syntax.
type DropSinkStmt struct {
	Sink StreamIdentifier
}

func (s DropSinkStmt) String() string {
	str := []string{"DROP", "SINK", string(s.Sink)}
	return strings.Join(str, " ")
}

// DropStateStmt represents DROP STATE .. syntax.
type DropStateStmt struct {
	State StreamIdentifier
}

func (s DropStateStmt) String() string {
	str := []string{"DROP", "STATE", string(s.State)}
	return strings.Join(str, " ")
}

// LoadStateStmt represents LOAD STATE .. syntax.
type LoadStateStmt struct {
	Name StreamIdentifier
	Type SourceSinkType
	Tag  string
	SourceSinkSpecsAST
}

func (s LoadStateStmt) String() string {
	str := []string{"LOAD", "STATE", string(s.Name), "TYPE", string(s.Type)}
	if s.Tag != "" {
		str = append(str, "TAG", s.Tag)
	}
	specs := s.SourceSinkSpecsAST.string("SET")
	if specs != "" {
		str = append(str, specs)
	}
	return strings.Join(str, " ")
}

// LoadStateOrCreateStmt represents LOAD STATE .. OR CREATE .. syntax.
type LoadStateOrCreateStmt struct {
	Name        StreamIdentifier
	Type        SourceSinkType
	Tag         string
	LoadSpecs   SourceSinkSpecsAST
	CreateSpecs SourceSinkSpecsAST
}

func (s LoadStateOrCreateStmt) String() string {
	str := []string{"LOAD", "STATE", string(s.Name), "TYPE", string(s.Type)}
	if s.Tag != "" {
		str = append(str, "TAG", s.Tag)
	}
	specs := s.LoadSpecs.string("SET")
	if specs != "" {
		str = append(str, specs)
	}

	str = append(str, "OR CREATE IF NOT SAVED")

	createSpecs := s.CreateSpecs.string("WITH")
	if createSpecs != "" {
		str = append(str, createSpecs)
	}
	return strings.Join(str, " ")
}

// SaveStateStmt represents SAVE STATE .. syntax.
type SaveStateStmt struct {
	Name StreamIdentifier
	Tag  string
}

func (s SaveStateStmt) String() string {
	str := []string{"SAVE", "STATE", string(s.Name)}
	if s.Tag != "" {
		str = append(str, "TAG", s.Tag)
	}
	return strings.Join(str, " ")
}

// EvalStmt represents EVAL .. syntax.
type EvalStmt struct {
	Expr  Expression
	Input *MapAST
}

func (s EvalStmt) String() string {
	str := []string{"EVAL", s.Expr.String()}
	if s.Input != nil {
		str = append(str, "ON", s.Input.String())
	}
	return strings.Join(str, " ")
}

// EmitterAST represents a part of emission, [RANGE ..].
type EmitterAST struct {
	EmitterType    Emitter
	EmitterOptions []interface{}
}

func (a EmitterAST) string() string {
	s := a.EmitterType.String()
	if len(a.EmitterOptions) > 0 {
		optStrings := make([]string, len(a.EmitterOptions))
		for i, opt := range a.EmitterOptions {
			switch obj := opt.(type) {
			case EmitterLimit:
				optStrings[i] = fmt.Sprintf("LIMIT %d", obj.Limit)
			case EmitterSampling:
				optStrings[i] = obj.string()
			}
		}
		s += " [" + strings.Join(optStrings, " ") + "]"
	}
	return s
}

// EmitterLimit represents a part of emission with limit, [LIMIT ..].
type EmitterLimit struct {
	Limit int64
}

// EmitterSampling represents a part of emission with sampling, [EVERY ..].
type EmitterSampling struct {
	Value float64
	Type  EmitterSamplingType
}

func (e EmitterSampling) string() string {
	if e.Type == CountBasedSampling {
		countWord := "TH"
		switch int64(e.Value) {
		case 1:
			countWord = "ST"
		case 2:
			countWord = "ND"
		case 3:
			countWord = "RD"
		}
		return fmt.Sprintf("EVERY %d-%s TUPLE", int64(e.Value), countWord)
	} else if e.Type == RandomizedSampling {
		return fmt.Sprintf("SAMPLE %v%%", e.Value)
	} else if e.Type == TimeBasedSampling {
		if e.Value < 1 {
			return fmt.Sprintf("EVERY %v MILLISECONDS", e.Value*1000)
		}
		return fmt.Sprintf("EVERY %v SECONDS", e.Value)
	}
	return ""
}

// ProjectionsAST represents emission values.
type ProjectionsAST struct {
	Projections []Expression
}

func (a ProjectionsAST) string() string {
	prj := []string{}
	for _, e := range a.Projections {
		prj = append(prj, e.String())
	}
	return strings.Join(prj, ", ")
}

// AliasAST represents an alias of a value, .. AS ..
type AliasAST struct {
	Expr  Expression
	Alias string
}

// ReferencedRelations returns a values to be emitted.
func (a AliasAST) ReferencedRelations() map[string]bool {
	return a.Expr.ReferencedRelations()
}

// RenameReferencedRelation returns an expression, represented aliased value.
func (a AliasAST) RenameReferencedRelation(from, to string) Expression {
	return AliasAST{a.Expr.RenameReferencedRelation(from, to), a.Alias}
}

// Foldable returns the tree is fold-able or not.
func (a AliasAST) Foldable() bool {
	return a.Expr.Foldable()
}

// String returns a syntax of aliased value.
func (a AliasAST) String() string {
	return a.Expr.String() + " AS " + a.Alias
}

// WindowedFromAST represents data sources, .. FROM ..
type WindowedFromAST struct {
	Relations []AliasedStreamWindowAST
}

func (a WindowedFromAST) string() string {
	if len(a.Relations) == 0 {
		return ""
	}

	str := []string{}
	for _, r := range a.Relations {
		str = append(str, r.string())
	}
	return "FROM " + strings.Join(str, ", ")
}

// AliasedStreamWindowAST represented data sources with alias, .. FROM .. AS ..
type AliasedStreamWindowAST struct {
	StreamWindowAST
	Alias string
}

func (a AliasedStreamWindowAST) string() string {
	str := a.StreamWindowAST.string()
	if a.Alias != "" {
		str = str + " AS " + a.Alias
	}
	return str
}

// UnspecifiedCapacity is an initialized value for emission capacity, means
// not setup.
const UnspecifiedCapacity int64 = -1

type StreamWindowAST struct {
	Stream
	IntervalAST
	Capacity int64
	Shedding SheddingOption
}

func (a StreamWindowAST) string() string {
	interval := a.IntervalAST.string()
	capacity := ""
	if a.Capacity != UnspecifiedCapacity {
		capacity = fmt.Sprintf(", BUFFER SIZE %d", a.Capacity)
	}
	shedding := ""
	if a.Shedding != UnspecifiedSheddingOption {
		shedding = fmt.Sprintf(", %s IF FULL", a.Shedding.String())
	}
	suffix := "[" + interval + capacity + shedding + "]"

	switch a.Stream.Type {
	case ActualStream:
		return a.Stream.Name + " " + suffix

	case UDSFStream:
		ps := []string{}
		for _, p := range a.Stream.Params {
			ps = append(ps, p.String())
		}
		return a.Stream.Name + "(" + strings.Join(ps, ", ") + ") " + suffix
	}

	return "UnknownStreamType"
}

type IntervalAST struct {
	FloatLiteral
	Unit IntervalUnit
}

func (a IntervalAST) string() string {
	return "RANGE " + a.FloatLiteral.String() + " " + a.Unit.String()
}

type FilterAST struct {
	Filter Expression
}

func (a FilterAST) string() string {
	if a.Filter == nil {
		return ""
	}
	return "WHERE " + a.Filter.String()
}

type GroupingAST struct {
	GroupList []Expression
}

func (a GroupingAST) string() string {
	if len(a.GroupList) == 0 {
		return ""
	}

	str := []string{}
	for _, e := range a.GroupList {
		str = append(str, e.String())
	}
	return "GROUP BY " + strings.Join(str, ", ")
}

type HavingAST struct {
	Having Expression
}

func (a HavingAST) string() string {
	if a.Having == nil {
		return ""
	}
	return "HAVING " + a.Having.String()
}

type SourceSinkSpecsAST struct {
	Params []SourceSinkParamAST
}

func (a SourceSinkSpecsAST) string(keyword string) string {
	if len(a.Params) == 0 {
		return ""
	}
	ps := make([]string, len(a.Params))
	for i, p := range a.Params {
		ps[i] = p.string()
	}
	return keyword + " " + strings.Join(ps, ", ")
}

type SourceSinkParamAST struct {
	Key   SourceSinkParamKey
	Value data.Value
}

func (a SourceSinkParamAST) string() string {
	// helper function to convert to string and escape
	// actual data.String objects correctly
	mkString := func(v data.Value) string {
		s, _ := data.ToString(v)
		if v.Type() == data.TypeString {
			return StringLiteral{Value: s}.String()
		}
		return s
	}
	var valRepr string
	if a.Value.Type() == data.TypeArray {
		// convert arrays to string elementwise and
		// add brackets
		arr, _ := data.AsArray(a.Value)
		reps := make([]string, len(arr))
		for i, v := range arr {
			reps[i] = mkString(v)
		}
		valRepr = "[" + strings.Join(reps, ",") + "]"
	} else if a.Value.Type() == data.TypeMap {
		m, _ := data.AsMap(a.Value)
		ret := make([]string, len(m))
		i := 0
		for k, v := range m {
			ret[i] = StringLiteral{Value: k}.String() + ":" + mkString(v)
			i++
		}
		valRepr = "{" + strings.Join(ret, ",") + "}"
	} else {
		valRepr = mkString(a.Value)
	}
	return string(a.Key) + "=" + valRepr
}

type BinaryOpAST struct {
	Op    Operator
	Left  Expression
	Right Expression
}

func (b BinaryOpAST) ReferencedRelations() map[string]bool {
	rels := b.Left.ReferencedRelations()
	if rels == nil {
		return b.Right.ReferencedRelations()
	}
	for rel := range b.Right.ReferencedRelations() {
		rels[rel] = true
	}
	return rels
}

func (b BinaryOpAST) RenameReferencedRelation(from, to string) Expression {
	return BinaryOpAST{b.Op,
		b.Left.RenameReferencedRelation(from, to),
		b.Right.RenameReferencedRelation(from, to)}
}

func (b BinaryOpAST) Foldable() bool {
	return b.Left.Foldable() && b.Right.Foldable()
}

func (b BinaryOpAST) String() string {
	str := []string{b.Left.String(), b.Op.String(), b.Right.String()}

	// TODO: This implementation may add unnecessary parentheses.
	// For example, in
	//  input:  "a * 2 / b"
	//  output: "(a * 2) / b"
	// we could omit output parentehsis.

	// Enclose expression in parentheses for operator precedence
	encloseLeft, encloseRight := false, false

	if left, ok := b.Left.(BinaryOpAST); ok {
		if left.Op.hasHigherPrecedenceThan(b.Op) {
			// we need no parentheses
		} else {
			// we probably need parentheses
			encloseLeft = true
		}
	}

	if right, ok := b.Right.(BinaryOpAST); ok {
		if right.Op.hasHigherPrecedenceThan(b.Op) {
			// we need no parentheses
		} else {
			// we probably need parentheses
			encloseRight = true
		}
	}

	if encloseLeft {
		str[0] = "(" + str[0] + ")"
	}
	if encloseRight {
		str[2] = "(" + str[2] + ")"
	}

	return strings.Join(str, " ")
}

type UnaryOpAST struct {
	Op   Operator
	Expr Expression
}

func (u UnaryOpAST) ReferencedRelations() map[string]bool {
	return u.Expr.ReferencedRelations()
}

func (u UnaryOpAST) RenameReferencedRelation(from, to string) Expression {
	return UnaryOpAST{u.Op,
		u.Expr.RenameReferencedRelation(from, to)}
}

func (u UnaryOpAST) Foldable() bool {
	return u.Expr.Foldable()
}

func (u UnaryOpAST) String() string {
	op := u.Op.String()
	expr := u.Expr.String()

	// Unary minus operator such as "- - 2"
	if u.Op != UnaryMinus || strings.HasPrefix(expr, "-") {
		op = op + " "
	}

	// Enclose expression in parentheses for "NOT (a AND B)" like case
	if _, ok := u.Expr.(BinaryOpAST); ok {
		expr = "(" + expr + ")"
	}

	return op + expr
}

type TypeCastAST struct {
	Expr   Expression
	Target Type
}

func (u TypeCastAST) ReferencedRelations() map[string]bool {
	return u.Expr.ReferencedRelations()
}

func (u TypeCastAST) RenameReferencedRelation(from, to string) Expression {
	return TypeCastAST{u.Expr.RenameReferencedRelation(from, to),
		u.Target}
}

func (u TypeCastAST) Foldable() bool {
	return u.Expr.Foldable()
}

func (u TypeCastAST) String() string {
	if rv, ok := u.Expr.(RowValue); ok {
		return rv.String() + "::" + u.Target.String()
	}

	if rm, ok := u.Expr.(RowMeta); ok {
		return rm.String() + "::" + u.Target.String()
	}

	return "CAST(" + u.Expr.String() + " AS " + u.Target.String() + ")"
}

type FuncAppAST struct {
	Function FuncName
	ExpressionsAST
	Ordering []SortedExpressionAST
}

func (f FuncAppAST) ReferencedRelations() map[string]bool {
	rels := map[string]bool{}
	for _, expr := range f.Expressions {
		for rel := range expr.ReferencedRelations() {
			rels[rel] = true
		}
	}
	for _, expr := range f.Ordering {
		for rel := range expr.ReferencedRelations() {
			rels[rel] = true
		}
	}
	return rels
}

func (f FuncAppAST) RenameReferencedRelation(from, to string) Expression {
	newExprs := make([]Expression, len(f.Expressions))
	for i, expr := range f.Expressions {
		newExprs[i] = expr.RenameReferencedRelation(from, to)
	}
	newOrderExprs := make([]SortedExpressionAST, len(f.Ordering))
	for i, expr := range f.Ordering {
		newOrderExprs[i] = expr.RenameReferencedRelation(from, to).(SortedExpressionAST)
	}
	return FuncAppAST{f.Function, ExpressionsAST{newExprs}, newOrderExprs}
}

func (f FuncAppAST) Foldable() bool {
	foldable := true
	// now() is not evaluable outside of some execution context
	if string(f.Function) == "now" && len(f.Expressions) == 0 {
		return false
	}
	// if there is a ORDER BY clause, then this is definitely an
	// aggregate function and therefore not foldable
	if len(f.Ordering) > 0 {
		return false
	}
	for _, expr := range f.Expressions {
		if !expr.Foldable() {
			foldable = false
			break
		}
	}
	return foldable
}

func (f FuncAppAST) String() string {
	s := string(f.Function) + "(" + f.ExpressionsAST.string()
	if len(f.Ordering) > 0 {
		orderStrings := make([]string, len(f.Ordering))
		for i, expr := range f.Ordering {
			orderStrings[i] = expr.String()
		}
		s += " ORDER BY " + strings.Join(orderStrings, ", ")
	}
	return s + ")"
}

type SortedExpressionAST struct {
	Expr      Expression
	Ascending BinaryKeyword
}

func (s SortedExpressionAST) ReferencedRelations() map[string]bool {
	return s.Expr.ReferencedRelations()
}

func (s SortedExpressionAST) RenameReferencedRelation(from, to string) Expression {
	return SortedExpressionAST{s.Expr.RenameReferencedRelation(from, to),
		s.Ascending}
}

func (s SortedExpressionAST) Foldable() bool {
	return s.Expr.Foldable()
}

func (s SortedExpressionAST) String() string {
	ret := s.Expr.String()
	if s.Ascending == Yes {
		ret += " ASC"
	} else if s.Ascending == No {
		ret += " DESC"
	}
	return ret
}

type ArrayAST struct {
	ExpressionsAST
}

func (a ArrayAST) ReferencedRelations() map[string]bool {
	rels := map[string]bool{}
	for _, expr := range a.Expressions {
		for rel := range expr.ReferencedRelations() {
			rels[rel] = true
		}
	}
	return rels
}

func (a ArrayAST) RenameReferencedRelation(from, to string) Expression {
	newExprs := make([]Expression, len(a.Expressions))
	for i, expr := range a.Expressions {
		newExprs[i] = expr.RenameReferencedRelation(from, to)
	}
	return ArrayAST{ExpressionsAST{newExprs}}
}

func (a ArrayAST) Foldable() bool {
	foldable := true
	for _, expr := range a.Expressions {
		if !expr.Foldable() {
			foldable = false
			break
		}
	}
	return foldable
}

func (a ArrayAST) String() string {
	return "[" + a.ExpressionsAST.string() + "]"
}

type ExpressionsAST struct {
	Expressions []Expression
}

func (a ExpressionsAST) string() string {
	str := []string{}
	for _, e := range a.Expressions {
		str = append(str, e.String())
	}
	return strings.Join(str, ", ")
}

type MapAST struct {
	Entries []KeyValuePairAST
}

func (m MapAST) ReferencedRelations() map[string]bool {
	rels := map[string]bool{}
	for _, pair := range m.Entries {
		for rel := range pair.Value.ReferencedRelations() {
			rels[rel] = true
		}
	}
	return rels
}

func (m MapAST) RenameReferencedRelation(from, to string) Expression {
	newEntries := make([]KeyValuePairAST, len(m.Entries))
	for i, pair := range m.Entries {
		newEntries[i] = KeyValuePairAST{
			pair.Key,
			pair.Value.RenameReferencedRelation(from, to),
		}
	}
	return MapAST{newEntries}
}

func (m MapAST) Foldable() bool {
	foldable := true
	for _, pair := range m.Entries {
		if !pair.Value.Foldable() {
			foldable = false
			break
		}
	}
	return foldable
}

func (m MapAST) String() string {
	entries := []string{}
	for _, pair := range m.Entries {
		entries = append(entries, pair.string())
	}
	return "{" + strings.Join(entries, ", ") + "}"
}

type KeyValuePairAST struct {
	Key   string
	Value Expression
}

func (k KeyValuePairAST) string() string {
	return `"` + k.Key + `":` + k.Value.String()
}

// Elementary Structures (all without *AST for now)

// Note that we need the constructors for the elementary structures
// because we cannot use curly brackets for Expr{...} style
// initialization in the .peg file.

// It seems not possible in Go to have a variable that says "this is
// either struct A or struct B or struct C", so we build one struct
// that serves both for "real" streams (as in `FROM x`) and stream-
// generating functions (as in `FROM series(1, 5)`).
type Stream struct {
	Type   StreamType
	Name   string
	Params []Expression
}

func NewStream(s string) Stream {
	return Stream{ActualStream, s, nil}
}

type Wildcard struct {
	Relation string
}

func (w Wildcard) ReferencedRelations() map[string]bool {
	if w.Relation == "" {
		// the wildcard does not reference any relation
		// (this is different to referencing the "" relation)
		return nil
	}
	return map[string]bool{w.Relation: true}
}

func (w Wildcard) RenameReferencedRelation(from, to string) Expression {
	if w.Relation == from {
		return Wildcard{to}
	}
	return Wildcard{w.Relation}
}

func (w Wildcard) Foldable() bool {
	return false
}

func NewWildcard(relation string) Wildcard {
	return Wildcard{strings.TrimRight(relation, ":*")}
}

func (w Wildcard) String() string {
	if w.Relation != "" {
		return w.Relation + ":*"
	}
	return "*"
}

type RowValue struct {
	Relation string
	Column   string
}

func (rv RowValue) ReferencedRelations() map[string]bool {
	return map[string]bool{rv.Relation: true}
}

func (rv RowValue) RenameReferencedRelation(from, to string) Expression {
	if rv.Relation == from {
		return RowValue{to, rv.Column}
	}
	return rv
}

func (rv RowValue) Foldable() bool {
	return false
}

func (rv RowValue) String() string {
	if rv.Relation != "" {
		return rv.Relation + ":" + rv.Column
	}
	return rv.Column
}

func NewRowValue(s string) RowValue {
	bracketPos := strings.Index(s, "[")
	components := strings.SplitN(s, ":", 2)
	if bracketPos >= 0 && bracketPos < len(components[0]) {
		// if there is a bracket, then it is definitely on the right
		// side of the colon. therefore, if the part before the first
		// found colon is longer than where the first bracket is,
		// then the colon is part of the JSON path, not the stream
		// separator.
		return RowValue{"", s}
	} else if len(components) == 1 {
		// just "col"
		return RowValue{"", components[0]}
	}
	// "table.col"
	return RowValue{components[0], components[1]}
}

type WhenThenPairAST struct {
	When Expression
	Then Expression
}

func (wt WhenThenPairAST) string() string {
	return fmt.Sprintf("WHEN %s THEN %s", wt.When.String(), wt.Then.String())
}

type ConditionCaseAST struct {
	Checks []WhenThenPairAST
	Else   Expression
}

func (c ConditionCaseAST) ReferencedRelations() map[string]bool {
	rels := map[string]bool{}
	for _, pair := range c.Checks {
		for rel := range pair.When.ReferencedRelations() {
			rels[rel] = true
		}
		for rel := range pair.Then.ReferencedRelations() {
			rels[rel] = true
		}
	}
	if c.Else != nil {
		for rel := range c.Else.ReferencedRelations() {
			rels[rel] = true
		}
	}
	return rels
}

func (c ConditionCaseAST) RenameReferencedRelation(from, to string) Expression {
	newChecks := make([]WhenThenPairAST, len(c.Checks))
	for i, pair := range c.Checks {
		newChecks[i] = WhenThenPairAST{
			pair.When.RenameReferencedRelation(from, to),
			pair.Then.RenameReferencedRelation(from, to),
		}
	}
	if c.Else != nil {
		return ConditionCaseAST{
			newChecks,
			c.Else.RenameReferencedRelation(from, to),
		}
	}
	return ConditionCaseAST{
		newChecks,
		nil,
	}
}

func (c ConditionCaseAST) Foldable() bool {
	for _, pair := range c.Checks {
		if !pair.When.Foldable() || !pair.Then.Foldable() {
			return false
		}
	}
	if c.Else != nil && !c.Else.Foldable() {
		return false
	}
	return true
}

func (c ConditionCaseAST) String() string {
	entries := []string{}
	for _, pair := range c.Checks {
		entries = append(entries, pair.string())
	}
	if c.Else != nil {
		return fmt.Sprintf("CASE %s ELSE %s END",
			strings.Join(entries, " "), c.Else.String())
	}
	return fmt.Sprintf("CASE %s END",
		strings.Join(entries, " "))
}

type ExpressionCaseAST struct {
	Expr Expression
	ConditionCaseAST
}

func (c ExpressionCaseAST) ReferencedRelations() map[string]bool {
	rels := c.Expr.ReferencedRelations()
	for rel := range c.ConditionCaseAST.ReferencedRelations() {
		rels[rel] = true
	}
	return rels
}

func (c ExpressionCaseAST) RenameReferencedRelation(from, to string) Expression {
	return ExpressionCaseAST{
		c.Expr.RenameReferencedRelation(from, to),
		c.ConditionCaseAST.RenameReferencedRelation(from, to).(ConditionCaseAST),
	}
}

func (c ExpressionCaseAST) Foldable() bool {
	return c.Expr.Foldable() && c.ConditionCaseAST.Foldable()
}

func (c ExpressionCaseAST) String() string {
	entries := []string{}
	for _, pair := range c.Checks {
		entries = append(entries, pair.string())
	}
	if c.Else != nil {
		return fmt.Sprintf("CASE %s %s ELSE %s END",
			c.Expr.String(), strings.Join(entries, " "), c.Else.String())
	}
	return fmt.Sprintf("CASE %s %s END",
		c.Expr.String(), strings.Join(entries, " "))
}

type RowMeta struct {
	Relation string
	MetaType MetaInformation
}

func (rm RowMeta) ReferencedRelations() map[string]bool {
	return map[string]bool{rm.Relation: true}
}

func (rm RowMeta) RenameReferencedRelation(from, to string) Expression {
	if rm.Relation == from {
		return RowMeta{to, rm.MetaType}
	}
	return rm
}

func (rm RowMeta) Foldable() bool {
	return false
}

func (rm RowMeta) String() string {
	if rm.Relation != "" {
		return rm.Relation + ":" + rm.MetaType.string()
	}
	return rm.MetaType.string()
}

func NewRowMeta(s string, t MetaInformation) RowMeta {
	components := strings.SplitN(s, ":", 2)
	if len(components) == 1 {
		// just the meta information
		return RowMeta{"", t}
	}
	// relation name and meta information
	return RowMeta{components[0], t}
}

type Raw struct {
	Expr string
}

func NewRaw(s string) Raw {
	return Raw{s}
}

type NumericLiteral struct {
	Value int64
}

func (l NumericLiteral) ReferencedRelations() map[string]bool {
	return nil
}

func (l NumericLiteral) RenameReferencedRelation(from, to string) Expression {
	return l
}

func (l NumericLiteral) Foldable() bool {
	return true
}

func (l NumericLiteral) String() string {
	return fmt.Sprintf("%v", l.Value)
}

func NewNumericLiteral(s string) NumericLiteral {
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		panic(err)
	}
	return NumericLiteral{val}
}

type FloatLiteral struct {
	Value float64
}

func (l FloatLiteral) ReferencedRelations() map[string]bool {
	return nil
}

func (l FloatLiteral) RenameReferencedRelation(from, to string) Expression {
	return l
}

func (l FloatLiteral) Foldable() bool {
	return true
}

func (l FloatLiteral) String() string {
	return fmt.Sprintf("%v", l.Value)
}

func NewFloatLiteral(s string) FloatLiteral {
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		panic(err)
	}
	return FloatLiteral{val}
}

type NullLiteral struct {
}

func (l NullLiteral) ReferencedRelations() map[string]bool {
	return nil
}

func (l NullLiteral) RenameReferencedRelation(from, to string) Expression {
	return l
}

func (l NullLiteral) Foldable() bool {
	return true
}

func (l NullLiteral) String() string {
	return "NULL"
}

func NewNullLiteral() NullLiteral {
	return NullLiteral{}
}

type Missing struct {
}

func (l Missing) ReferencedRelations() map[string]bool {
	return nil
}

func (l Missing) RenameReferencedRelation(from, to string) Expression {
	return l
}

func (l Missing) Foldable() bool {
	return true
}

func (l Missing) String() string {
	return "MISSING"
}

func NewMissing() Missing {
	return Missing{}
}

type BoolLiteral struct {
	Value bool
}

func (l BoolLiteral) ReferencedRelations() map[string]bool {
	return nil
}

func (l BoolLiteral) RenameReferencedRelation(from, to string) Expression {
	return l
}

func (l BoolLiteral) Foldable() bool {
	return true
}

func (l BoolLiteral) String() string {
	if l.Value {
		return "TRUE"
	}
	return "FALSE"
}

func NewBoolLiteral(b bool) BoolLiteral {
	return BoolLiteral{b}
}

type StringLiteral struct {
	Value string
}

func (l StringLiteral) ReferencedRelations() map[string]bool {
	return nil
}

func (l StringLiteral) RenameReferencedRelation(from, to string) Expression {
	return l
}

func (l StringLiteral) Foldable() bool {
	return true
}

func (l StringLiteral) String() string {
	return `"` + strings.Replace(l.Value, `"`, `""`, -1) + `"`
}

func NewStringLiteral(s string) StringLiteral {
	runes := []rune(s)
	stripped := string(runes[1 : len(runes)-1])
	unescaped := strings.Replace(stripped, `""`, `"`, -1)
	return StringLiteral{unescaped}
}

type FuncName string

type StreamIdentifier string

type SourceSinkType string

type SourceSinkParamKey string

type Emitter int

const (
	UnspecifiedEmitter Emitter = iota
	Istream
	Dstream
	Rstream
)

func (e Emitter) String() string {
	s := "UNSPECIFIED"
	switch e {
	case Istream:
		s = "ISTREAM"
	case Dstream:
		s = "DSTREAM"
	case Rstream:
		s = "RSTREAM"
	}
	return s
}

type EmitterSamplingType int

const (
	UnspecifiedSamplingType EmitterSamplingType = iota
	CountBasedSampling
	RandomizedSampling
	TimeBasedSampling
)

func (est EmitterSamplingType) String() string {
	s := "UNKNOWN"
	switch est {
	case CountBasedSampling:
		s = "EVERY k-TH TUPLE"
	case RandomizedSampling:
		s = "SAMPLE"
	case TimeBasedSampling:
		s = "EVERY k SECONDS"
	}
	return s
}

type StreamType int

const (
	UnknownStreamType StreamType = iota
	ActualStream
	UDSFStream
)

func (st StreamType) String() string {
	s := "UNKNOWN"
	switch st {
	case ActualStream:
		s = "ActualStream"
	case UDSFStream:
		s = "UDSFStream"
	}
	return s
}

type IntervalUnit int

const (
	UnspecifiedIntervalUnit IntervalUnit = iota
	Tuples
	Seconds
	Milliseconds
)

func (i IntervalUnit) String() string {
	s := "UNSPECIFIED"
	switch i {
	case Tuples:
		s = "TUPLES"
	case Seconds:
		s = "SECONDS"
	case Milliseconds:
		s = "MILLISECONDS"
	}
	return s
}

type MetaInformation int

const (
	UnknownMeta MetaInformation = iota
	TimestampMeta
	NowMeta
)

func (m MetaInformation) String() string {
	s := "UnknownMeta"
	switch m {
	case TimestampMeta:
		s = "TS"
	case NowMeta:
		s = "NOW"
	}
	return s
}

func (m MetaInformation) string() string {
	s := "UnknownMeta"
	switch m {
	case TimestampMeta:
		s = "ts()"
	case NowMeta:
		s = "now()"
	}
	return s
}

type BinaryKeyword int

const (
	UnspecifiedKeyword BinaryKeyword = iota
	Yes
	No
)

func (k BinaryKeyword) String() string {
	s := "Unspecified"
	switch k {
	case Yes:
		s = "Yes"
	case No:
		s = "No"
	}
	return s
}

func (k BinaryKeyword) string(yes, no string) string {
	switch k {
	case Yes:
		return yes
	case No:
		return no
	}
	return ""
}

type SheddingOption int

const (
	UnspecifiedSheddingOption SheddingOption = iota
	Wait
	DropOldest
	DropNewest
)

func (t SheddingOption) String() string {
	s := "UnspecifiedSheddingOption"
	switch t {
	case Wait:
		s = "WAIT"
	case DropOldest:
		s = "DROP OLDEST"
	case DropNewest:
		s = "DROP NEWEST"
	}
	return s
}

type Type int

const (
	UnknownType Type = iota
	Bool
	Int
	Float
	String
	Blob
	Timestamp
	Array
	Map
)

func (t Type) String() string {
	s := "UnknownType"
	switch t {
	case Bool:
		s = "BOOL"
	case Int:
		s = "INT"
	case Float:
		s = "FLOAT"
	case String:
		s = "STRING"
	case Blob:
		s = "BLOB"
	case Timestamp:
		s = "TIMESTAMP"
	case Array:
		s = "ARRAY"
	case Map:
		s = "MAP"
	}
	return s
}

type Operator int

const (
	// Operators are defined in precedence order (increasing). These
	// values can be compared using the hasHigherPrecedenceThan method.
	UnknownOperator Operator = iota
	Or
	And
	Not
	Equal
	Less
	LessOrEqual
	Greater
	GreaterOrEqual
	NotEqual
	Concat
	Is
	IsNot
	Plus
	Minus
	Multiply
	Divide
	Modulo
	UnaryMinus
)

// hasSamePrecedenceAs checks if the arguement operator has the same precedence.
func (op Operator) hasSamePrecedenceAs(rhs Operator) bool {
	if Or <= op && op <= Not && Or <= rhs && rhs <= Not {
		return true
	}
	if Less <= op && op <= GreaterOrEqual && Less <= rhs && rhs <= GreaterOrEqual {
		return true
	}
	if Is <= op && op <= IsNot && Is <= rhs && rhs <= IsNot {
		return true
	}
	if Plus <= op && op <= Minus && Plus <= rhs && rhs <= Minus {
		return true
	}
	if Multiply <= op && op <= Modulo && Multiply <= rhs && rhs <= Modulo {
		return true
	}

	return false
}

func (op Operator) hasHigherPrecedenceThan(rhs Operator) bool {
	if op.hasSamePrecedenceAs(rhs) {
		return false
	}

	return op > rhs
}

func (o Operator) String() string {
	s := "UnknownOperator"
	switch o {
	case Or:
		s = "OR"
	case And:
		s = "AND"
	case Not:
		s = "NOT"
	case Equal:
		s = "="
	case Less:
		s = "<"
	case LessOrEqual:
		s = "<="
	case Greater:
		s = ">"
	case GreaterOrEqual:
		s = ">="
	case NotEqual:
		s = "!="
	case Concat:
		s = "||"
	case Is:
		s = "IS"
	case IsNot:
		s = "IS NOT"
	case Plus:
		s = "+"
	case Minus:
		s = "-"
	case Multiply:
		s = "*"
	case Divide:
		s = "/"
	case Modulo:
		s = "%"
	case UnaryMinus:
		s = "-"
	}
	return s
}

type Identifier string
