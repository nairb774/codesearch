// Code generated by the FlatBuffers compiler. DO NOT EDIT.

package index

import (
	"strconv"

	flatbuffers "github.com/google/flatbuffers/go"
)

type DocType int8

const (
	DocTypeRegular    DocType = 0
	DocTypeExecutable DocType = 1
	DocTypeSymlink    DocType = 2
)

var EnumNamesDocType = map[DocType]string{
	DocTypeRegular:    "Regular",
	DocTypeExecutable: "Executable",
	DocTypeSymlink:    "Symlink",
}

var EnumValuesDocType = map[string]DocType{
	"Regular":    DocTypeRegular,
	"Executable": DocTypeExecutable,
	"Symlink":    DocTypeSymlink,
}

func (v DocType) String() string {
	if s, ok := EnumNamesDocType[v]; ok {
		return s
	}
	return "DocType(" + strconv.FormatInt(int64(v), 10) + ")"
}

type GitHash struct {
	_tab flatbuffers.Struct
}

func (rcv *GitHash) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *GitHash) Table() flatbuffers.Table {
	return rcv._tab.Table
}

func (rcv *GitHash) A() uint32 {
	return rcv._tab.GetUint32(rcv._tab.Pos + flatbuffers.UOffsetT(0))
}
func (rcv *GitHash) MutateA(n uint32) bool {
	return rcv._tab.MutateUint32(rcv._tab.Pos+flatbuffers.UOffsetT(0), n)
}

func (rcv *GitHash) B() uint32 {
	return rcv._tab.GetUint32(rcv._tab.Pos + flatbuffers.UOffsetT(4))
}
func (rcv *GitHash) MutateB(n uint32) bool {
	return rcv._tab.MutateUint32(rcv._tab.Pos+flatbuffers.UOffsetT(4), n)
}

func (rcv *GitHash) C() uint32 {
	return rcv._tab.GetUint32(rcv._tab.Pos + flatbuffers.UOffsetT(8))
}
func (rcv *GitHash) MutateC(n uint32) bool {
	return rcv._tab.MutateUint32(rcv._tab.Pos+flatbuffers.UOffsetT(8), n)
}

func (rcv *GitHash) D() uint32 {
	return rcv._tab.GetUint32(rcv._tab.Pos + flatbuffers.UOffsetT(12))
}
func (rcv *GitHash) MutateD(n uint32) bool {
	return rcv._tab.MutateUint32(rcv._tab.Pos+flatbuffers.UOffsetT(12), n)
}

func (rcv *GitHash) E() uint32 {
	return rcv._tab.GetUint32(rcv._tab.Pos + flatbuffers.UOffsetT(16))
}
func (rcv *GitHash) MutateE(n uint32) bool {
	return rcv._tab.MutateUint32(rcv._tab.Pos+flatbuffers.UOffsetT(16), n)
}

func CreateGitHash(builder *flatbuffers.Builder, a uint32, b uint32, c uint32, d uint32, e uint32) flatbuffers.UOffsetT {
	builder.Prep(4, 20)
	builder.PrependUint32(e)
	builder.PrependUint32(d)
	builder.PrependUint32(c)
	builder.PrependUint32(b)
	builder.PrependUint32(a)
	return builder.Offset()
}
type Sha256 struct {
	_tab flatbuffers.Struct
}

func (rcv *Sha256) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *Sha256) Table() flatbuffers.Table {
	return rcv._tab.Table
}

func (rcv *Sha256) A() uint32 {
	return rcv._tab.GetUint32(rcv._tab.Pos + flatbuffers.UOffsetT(0))
}
func (rcv *Sha256) MutateA(n uint32) bool {
	return rcv._tab.MutateUint32(rcv._tab.Pos+flatbuffers.UOffsetT(0), n)
}

func (rcv *Sha256) B() uint32 {
	return rcv._tab.GetUint32(rcv._tab.Pos + flatbuffers.UOffsetT(4))
}
func (rcv *Sha256) MutateB(n uint32) bool {
	return rcv._tab.MutateUint32(rcv._tab.Pos+flatbuffers.UOffsetT(4), n)
}

func (rcv *Sha256) C() uint32 {
	return rcv._tab.GetUint32(rcv._tab.Pos + flatbuffers.UOffsetT(8))
}
func (rcv *Sha256) MutateC(n uint32) bool {
	return rcv._tab.MutateUint32(rcv._tab.Pos+flatbuffers.UOffsetT(8), n)
}

func (rcv *Sha256) D() uint32 {
	return rcv._tab.GetUint32(rcv._tab.Pos + flatbuffers.UOffsetT(12))
}
func (rcv *Sha256) MutateD(n uint32) bool {
	return rcv._tab.MutateUint32(rcv._tab.Pos+flatbuffers.UOffsetT(12), n)
}

func (rcv *Sha256) E() uint32 {
	return rcv._tab.GetUint32(rcv._tab.Pos + flatbuffers.UOffsetT(16))
}
func (rcv *Sha256) MutateE(n uint32) bool {
	return rcv._tab.MutateUint32(rcv._tab.Pos+flatbuffers.UOffsetT(16), n)
}

func (rcv *Sha256) F() uint32 {
	return rcv._tab.GetUint32(rcv._tab.Pos + flatbuffers.UOffsetT(20))
}
func (rcv *Sha256) MutateF(n uint32) bool {
	return rcv._tab.MutateUint32(rcv._tab.Pos+flatbuffers.UOffsetT(20), n)
}

func (rcv *Sha256) G() uint32 {
	return rcv._tab.GetUint32(rcv._tab.Pos + flatbuffers.UOffsetT(24))
}
func (rcv *Sha256) MutateG(n uint32) bool {
	return rcv._tab.MutateUint32(rcv._tab.Pos+flatbuffers.UOffsetT(24), n)
}

func (rcv *Sha256) H() uint32 {
	return rcv._tab.GetUint32(rcv._tab.Pos + flatbuffers.UOffsetT(28))
}
func (rcv *Sha256) MutateH(n uint32) bool {
	return rcv._tab.MutateUint32(rcv._tab.Pos+flatbuffers.UOffsetT(28), n)
}

func CreateSha256(builder *flatbuffers.Builder, a uint32, b uint32, c uint32, d uint32, e uint32, f uint32, g uint32, h uint32) flatbuffers.UOffsetT {
	builder.Prep(4, 32)
	builder.PrependUint32(h)
	builder.PrependUint32(g)
	builder.PrependUint32(f)
	builder.PrependUint32(e)
	builder.PrependUint32(d)
	builder.PrependUint32(c)
	builder.PrependUint32(b)
	builder.PrependUint32(a)
	return builder.Offset()
}
type ByteRange struct {
	_tab flatbuffers.Struct
}

func (rcv *ByteRange) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *ByteRange) Table() flatbuffers.Table {
	return rcv._tab.Table
}

func (rcv *ByteRange) Start() uint64 {
	return rcv._tab.GetUint64(rcv._tab.Pos + flatbuffers.UOffsetT(0))
}
func (rcv *ByteRange) MutateStart(n uint64) bool {
	return rcv._tab.MutateUint64(rcv._tab.Pos+flatbuffers.UOffsetT(0), n)
}

func (rcv *ByteRange) Length() uint32 {
	return rcv._tab.GetUint32(rcv._tab.Pos + flatbuffers.UOffsetT(8))
}
func (rcv *ByteRange) MutateLength(n uint32) bool {
	return rcv._tab.MutateUint32(rcv._tab.Pos+flatbuffers.UOffsetT(8), n)
}

func CreateByteRange(builder *flatbuffers.Builder, start uint64, length uint32) flatbuffers.UOffsetT {
	builder.Prep(8, 16)
	builder.Pad(4)
	builder.PrependUint32(length)
	builder.PrependUint64(start)
	return builder.Offset()
}
type Doc struct {
	_tab flatbuffers.Table
}

func GetRootAsDoc(buf []byte, offset flatbuffers.UOffsetT) *Doc {
	n := flatbuffers.GetUOffsetT(buf[offset:])
	x := &Doc{}
	x.Init(buf, n+offset)
	return x
}

func (rcv *Doc) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *Doc) Table() flatbuffers.Table {
	return rcv._tab
}

func (rcv *Doc) Path() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *Doc) Range(obj *ByteRange) *ByteRange {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		x := o + rcv._tab.Pos
		if obj == nil {
			obj = new(ByteRange)
		}
		obj.Init(rcv._tab.Bytes, x)
		return obj
	}
	return nil
}

func (rcv *Doc) Size() uint32 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		return rcv._tab.GetUint32(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *Doc) MutateSize(n uint32) bool {
	return rcv._tab.MutateUint32Slot(8, n)
}

func (rcv *Doc) ModNs() int64 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(10))
	if o != 0 {
		return rcv._tab.GetInt64(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *Doc) MutateModNs(n int64) bool {
	return rcv._tab.MutateInt64Slot(10, n)
}

func (rcv *Doc) Type() DocType {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(12))
	if o != 0 {
		return DocType(rcv._tab.GetInt8(o + rcv._tab.Pos))
	}
	return 0
}

func (rcv *Doc) MutateType(n DocType) bool {
	return rcv._tab.MutateInt8Slot(12, int8(n))
}

func (rcv *Doc) Sha256(obj *Sha256) *Sha256 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(14))
	if o != 0 {
		x := o + rcv._tab.Pos
		if obj == nil {
			obj = new(Sha256)
		}
		obj.Init(rcv._tab.Bytes, x)
		return obj
	}
	return nil
}

func DocStart(builder *flatbuffers.Builder) {
	builder.StartObject(6)
}
func DocAddPath(builder *flatbuffers.Builder, path flatbuffers.UOffsetT) {
	builder.PrependUOffsetTSlot(0, flatbuffers.UOffsetT(path), 0)
}
func DocAddRange(builder *flatbuffers.Builder, range_ flatbuffers.UOffsetT) {
	builder.PrependStructSlot(1, flatbuffers.UOffsetT(range_), 0)
}
func DocAddSize(builder *flatbuffers.Builder, size uint32) {
	builder.PrependUint32Slot(2, size, 0)
}
func DocAddModNs(builder *flatbuffers.Builder, modNs int64) {
	builder.PrependInt64Slot(3, modNs, 0)
}
func DocAddType(builder *flatbuffers.Builder, type_ DocType) {
	builder.PrependInt8Slot(4, int8(type_), 0)
}
func DocAddSha256(builder *flatbuffers.Builder, sha256 flatbuffers.UOffsetT) {
	builder.PrependStructSlot(5, flatbuffers.UOffsetT(sha256), 0)
}
func DocEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT {
	return builder.EndObject()
}
type DocInner struct {
	_tab flatbuffers.Table
}

func GetRootAsDocInner(buf []byte, offset flatbuffers.UOffsetT) *DocInner {
	n := flatbuffers.GetUOffsetT(buf[offset:])
	x := &DocInner{}
	x.Init(buf, n+offset)
	return x
}

func (rcv *DocInner) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *DocInner) Table() flatbuffers.Table {
	return rcv._tab
}

func (rcv *DocInner) Hash(obj *GitHash) *GitHash {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		x := o + rcv._tab.Pos
		if obj == nil {
			obj = new(GitHash)
		}
		obj.Init(rcv._tab.Bytes, x)
		return obj
	}
	return nil
}

func (rcv *DocInner) BlockToLine(j int) uint32 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.GetUint32(a + flatbuffers.UOffsetT(j*4))
	}
	return 0
}

func (rcv *DocInner) BlockToLineLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func (rcv *DocInner) MutateBlockToLine(j int, n uint32) bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.MutateUint32(a+flatbuffers.UOffsetT(j*4), n)
	}
	return false
}

func (rcv *DocInner) Data(j int) byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.GetByte(a + flatbuffers.UOffsetT(j*1))
	}
	return 0
}

func (rcv *DocInner) DataLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func (rcv *DocInner) DataBytes() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *DocInner) MutateData(j int, n byte) bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.MutateByte(a+flatbuffers.UOffsetT(j*1), n)
	}
	return false
}

func DocInnerStart(builder *flatbuffers.Builder) {
	builder.StartObject(3)
}
func DocInnerAddHash(builder *flatbuffers.Builder, hash flatbuffers.UOffsetT) {
	builder.PrependStructSlot(0, flatbuffers.UOffsetT(hash), 0)
}
func DocInnerAddBlockToLine(builder *flatbuffers.Builder, blockToLine flatbuffers.UOffsetT) {
	builder.PrependUOffsetTSlot(1, flatbuffers.UOffsetT(blockToLine), 0)
}
func DocInnerStartBlockToLineVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT {
	return builder.StartVector(4, numElems, 4)
}
func DocInnerAddData(builder *flatbuffers.Builder, data flatbuffers.UOffsetT) {
	builder.PrependUOffsetTSlot(2, flatbuffers.UOffsetT(data), 0)
}
func DocInnerStartDataVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT {
	return builder.StartVector(1, numElems, 1)
}
func DocInnerEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT {
	return builder.EndObject()
}
type PostingList struct {
	_tab flatbuffers.Table
}

func GetRootAsPostingList(buf []byte, offset flatbuffers.UOffsetT) *PostingList {
	n := flatbuffers.GetUOffsetT(buf[offset:])
	x := &PostingList{}
	x.Init(buf, n+offset)
	return x
}

func (rcv *PostingList) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *PostingList) Table() flatbuffers.Table {
	return rcv._tab
}

func (rcv *PostingList) Docs(j int) byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.GetByte(a + flatbuffers.UOffsetT(j*1))
	}
	return 0
}

func (rcv *PostingList) DocsLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func (rcv *PostingList) DocsBytes() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *PostingList) MutateDocs(j int, n byte) bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.MutateByte(a+flatbuffers.UOffsetT(j*1), n)
	}
	return false
}

func PostingListStart(builder *flatbuffers.Builder) {
	builder.StartObject(1)
}
func PostingListAddDocs(builder *flatbuffers.Builder, docs flatbuffers.UOffsetT) {
	builder.PrependUOffsetTSlot(0, flatbuffers.UOffsetT(docs), 0)
}
func PostingListStartDocsVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT {
	return builder.StartVector(1, numElems, 1)
}
func PostingListEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT {
	return builder.EndObject()
}
type PostingLists struct {
	_tab flatbuffers.Table
}

func GetRootAsPostingLists(buf []byte, offset flatbuffers.UOffsetT) *PostingLists {
	n := flatbuffers.GetUOffsetT(buf[offset:])
	x := &PostingLists{}
	x.Init(buf, n+offset)
	return x
}

func (rcv *PostingLists) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *PostingLists) Table() flatbuffers.Table {
	return rcv._tab
}

func (rcv *PostingLists) Trigrams(j int) byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.GetByte(a + flatbuffers.UOffsetT(j*1))
	}
	return 0
}

func (rcv *PostingLists) TrigramsLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func (rcv *PostingLists) TrigramsBytes() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *PostingLists) MutateTrigrams(j int, n byte) bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.MutateByte(a+flatbuffers.UOffsetT(j*1), n)
	}
	return false
}

func (rcv *PostingLists) Lists(obj *PostingList, j int) bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		x := rcv._tab.Vector(o)
		x += flatbuffers.UOffsetT(j) * 4
		x = rcv._tab.Indirect(x)
		obj.Init(rcv._tab.Bytes, x)
		return true
	}
	return false
}

func (rcv *PostingLists) ListsLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func (rcv *PostingLists) MaxDocId() uint32 {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		return rcv._tab.GetUint32(o + rcv._tab.Pos)
	}
	return 0
}

func (rcv *PostingLists) MutateMaxDocId(n uint32) bool {
	return rcv._tab.MutateUint32Slot(8, n)
}

func PostingListsStart(builder *flatbuffers.Builder) {
	builder.StartObject(3)
}
func PostingListsAddTrigrams(builder *flatbuffers.Builder, trigrams flatbuffers.UOffsetT) {
	builder.PrependUOffsetTSlot(0, flatbuffers.UOffsetT(trigrams), 0)
}
func PostingListsStartTrigramsVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT {
	return builder.StartVector(1, numElems, 1)
}
func PostingListsAddLists(builder *flatbuffers.Builder, lists flatbuffers.UOffsetT) {
	builder.PrependUOffsetTSlot(1, flatbuffers.UOffsetT(lists), 0)
}
func PostingListsStartListsVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT {
	return builder.StartVector(4, numElems, 4)
}
func PostingListsAddMaxDocId(builder *flatbuffers.Builder, maxDocId uint32) {
	builder.PrependUint32Slot(2, maxDocId, 0)
}
func PostingListsEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT {
	return builder.EndObject()
}
type IndexShard struct {
	_tab flatbuffers.Table
}

func GetRootAsIndexShard(buf []byte, offset flatbuffers.UOffsetT) *IndexShard {
	n := flatbuffers.GetUOffsetT(buf[offset:])
	x := &IndexShard{}
	x.Init(buf, n+offset)
	return x
}

func (rcv *IndexShard) Init(buf []byte, i flatbuffers.UOffsetT) {
	rcv._tab.Bytes = buf
	rcv._tab.Pos = i
}

func (rcv *IndexShard) Table() flatbuffers.Table {
	return rcv._tab
}

func (rcv *IndexShard) Docs(obj *Doc, j int) bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		x := rcv._tab.Vector(o)
		x += flatbuffers.UOffsetT(j) * 4
		x = rcv._tab.Indirect(x)
		obj.Init(rcv._tab.Bytes, x)
		return true
	}
	return false
}

func (rcv *IndexShard) DocsLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(4))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func (rcv *IndexShard) Lists(obj *PostingLists) *PostingLists {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(6))
	if o != 0 {
		x := rcv._tab.Indirect(o + rcv._tab.Pos)
		if obj == nil {
			obj = new(PostingLists)
		}
		obj.Init(rcv._tab.Bytes, x)
		return obj
	}
	return nil
}

func (rcv *IndexShard) Raw(j int) byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.GetByte(a + flatbuffers.UOffsetT(j*1))
	}
	return 0
}

func (rcv *IndexShard) RawLength() int {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		return rcv._tab.VectorLen(o)
	}
	return 0
}

func (rcv *IndexShard) RawBytes() []byte {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		return rcv._tab.ByteVector(o + rcv._tab.Pos)
	}
	return nil
}

func (rcv *IndexShard) MutateRaw(j int, n byte) bool {
	o := flatbuffers.UOffsetT(rcv._tab.Offset(8))
	if o != 0 {
		a := rcv._tab.Vector(o)
		return rcv._tab.MutateByte(a+flatbuffers.UOffsetT(j*1), n)
	}
	return false
}

func IndexShardStart(builder *flatbuffers.Builder) {
	builder.StartObject(3)
}
func IndexShardAddDocs(builder *flatbuffers.Builder, docs flatbuffers.UOffsetT) {
	builder.PrependUOffsetTSlot(0, flatbuffers.UOffsetT(docs), 0)
}
func IndexShardStartDocsVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT {
	return builder.StartVector(4, numElems, 4)
}
func IndexShardAddLists(builder *flatbuffers.Builder, lists flatbuffers.UOffsetT) {
	builder.PrependUOffsetTSlot(1, flatbuffers.UOffsetT(lists), 0)
}
func IndexShardAddRaw(builder *flatbuffers.Builder, raw flatbuffers.UOffsetT) {
	builder.PrependUOffsetTSlot(2, flatbuffers.UOffsetT(raw), 0)
}
func IndexShardStartRawVector(builder *flatbuffers.Builder, numElems int) flatbuffers.UOffsetT {
	return builder.StartVector(1, numElems, 1)
}
func IndexShardEnd(builder *flatbuffers.Builder) flatbuffers.UOffsetT {
	return builder.EndObject()
}
