package index

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/RoaringBitmap/roaring"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/codesearch/sparse"
	flatbuffers "github.com/google/flatbuffers/go"
)

var (
	ErrInvalidUTF8     = errors.New("invalid utf-8 sequence")
	ErrLineTooLong     = errors.New("line too long")
	ErrFileTooBig      = errors.New("file too large")
	ErrTooManyTrigrams = errors.New("too many trigrams")
)

type FileContentsError struct {
	Msg error
}

func (f *FileContentsError) Error() string { return f.Msg.Error() }
func (f *FileContentsError) Unwrap() error { return f.Msg }

type IndexWriter struct {
	trigram *sparse.Set // trigrams for the current file

	sparseTrigrams []uint32
	denseTrigrams  []uint32
	lists          []*roaring.Bitmap

	docs []*docMeta

	raw       [][]byte
	rawLength int64
}

func (i *IndexWriter) Clear() {
	*i = IndexWriter{}
}

func (i *IndexWriter) DocCount() int { return len(i.docs) }

type docMeta struct {
	path string
	size int32
	mod  time.Time
	typ  DocType

	ref innerDocRef
}

type innerDocRef struct {
	sha256 SHA256
	start  int64
	length int32
}

func (i *IndexWriter) addFile(path string) (uint32, *docMeta) {
	m := &docMeta{
		path: path,
	}
	id := uint32(len(i.docs))
	i.docs = append(i.docs, m)
	return id, m
}

func (i *IndexWriter) addTrigrams(doc uint32) {
	if i.sparseTrigrams == nil {
		i.sparseTrigrams = make([]uint32, 1<<24)
	}

	for _, trigram := range i.trigram.Dense() {
		idx := i.sparseTrigrams[trigram]
		if n := uint32(len(i.denseTrigrams)); n <= idx || i.denseTrigrams[idx] != trigram {
			idx = n
			i.denseTrigrams = append(i.denseTrigrams, trigram)
			i.lists = append(i.lists, roaring.New())
			i.sparseTrigrams[trigram] = idx
		}
		i.lists[idx].Add(doc)
	}
}

func scanLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		return i + 1, data[:i+1], nil
	}

	if atEOF {
		if len(data) == 0 {
			data = nil
		}
		return len(data), data, nil
	}

	return 0, nil, nil
}

func computeTrigrams(set *sparse.Set, r io.Reader) ([]uint32, error) {
	s := bufio.NewScanner(r)
	s.Split(scanLines)

	var tok uint32
	var i int

	var line uint32
	var blockToLine []uint32
	for s.Scan() {
		line++

		if !utf8.Valid(s.Bytes()) {
			return nil, &FileContentsError{
				Msg: ErrInvalidUTF8,
			}
		}

		for _, b := range s.Bytes() {
			tok = ((tok & 0xFFFF) << 8) | uint32(b)
			if i++; i >= 3 {
				set.Add(tok)
			}
			// Every 4KiB blocks:
			if i&0xFFF == 0 {
				blockToLine = append(blockToLine, line)
			}
		}
	}

	err := s.Err()
	if err == bufio.ErrTooLong {
		err = &FileContentsError{
			Msg: ErrLineTooLong,
		}
	}

	return blockToLine, err
}

func (i *IndexWriter) addDocInnerToRaw(o docInnerW) innerDocRef {
	b := flatbuffers.NewBuilder(1024 + len(o.data) + len(o.blockToLine)*4)
	b.Finish(docInnerWrite(b, o))
	hash := sha256.Sum256(b.FinishedBytes())
	start, length := i.rawLength, int32(len(b.FinishedBytes()))
	i.raw = append(i.raw, b.FinishedBytes())
	i.rawLength += int64(length)
	return innerDocRef{
		sha256: hash,
		start:  start,
		length: length,
	}
}

func (i *IndexWriter) Add(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}

	if stat.Size() > 1<<30 {
		return &FileContentsError{
			Msg: ErrFileTooBig,
		}
	}

	if i.trigram == nil {
		i.trigram = sparse.NewSet(1 << 24)
	}
	i.trigram.Reset()
	var body bytes.Buffer
	blockToLine, err := computeTrigrams(i.trigram, io.TeeReader(f, &body))
	if err != nil {
		return err
	}

	ref := i.addDocInnerToRaw(docInnerW{
		blockToLine: blockToLine,
		data:        body.Bytes(),
	})

	id, doc := i.addFile(path)
	doc.size = int32(stat.Size())
	doc.mod = stat.ModTime()
	if mode := stat.Mode(); mode&os.ModeSymlink != 0 {
		doc.typ = DocTypeSymlink
	} else if mode&0o111 != 0 {
		doc.typ = DocTypeExecutable
	} else {
		doc.typ = DocTypeRegular
	}
	doc.ref = ref

	i.addTrigrams(id)

	return nil
}

func (i *IndexWriter) AddObject(lastEdit time.Time, f *object.File) error {
	var docType DocType
	switch f.Mode {
	case filemode.Regular:
		docType = DocTypeRegular
	case filemode.Executable:
		docType = DocTypeExecutable
	case filemode.Symlink:
		docType = DocTypeSymlink
	default:
		return nil
	}

	if f.Size > 1<<30 {
		return &FileContentsError{
			Msg: ErrFileTooBig,
		}
	}

	r, err := f.Reader()
	if err != nil {
		return err
	}
	defer r.Close()

	if i.trigram == nil {
		i.trigram = sparse.NewSet(1 << 24)
	}
	i.trigram.Reset()
	var body bytes.Buffer
	blockToLine, err := computeTrigrams(i.trigram, io.TeeReader(r, &body))
	if err != nil {
		return err
	}

	ref := i.addDocInnerToRaw(docInnerW{
		hash:        f.Hash,
		blockToLine: blockToLine,
		data:        body.Bytes(),
	})

	id, doc := i.addFile(f.Name)
	doc.size = int32(f.Size)
	doc.mod = lastEdit
	doc.typ = docType
	doc.ref = ref

	i.addTrigrams(id)

	return nil
}

func uint32Vector(builder *flatbuffers.Builder, v []uint32) flatbuffers.UOffsetT {
	builder.StartVector(4, len(v), 4)
	for idx := len(v) - 1; idx >= 0; idx-- {
		builder.PrependUint32(v[idx])
	}
	return builder.EndVector(len(v))
}

func uOffsetTVector(builder *flatbuffers.Builder, v []flatbuffers.UOffsetT) flatbuffers.UOffsetT {
	builder.StartVector(4, len(v), 4)
	for idx := len(v) - 1; idx >= 0; idx-- {
		builder.PrependUOffsetT(v[idx])
	}
	return builder.EndVector(len(v))
}

func gitHashWrite(builder *flatbuffers.Builder, hash plumbing.Hash) flatbuffers.UOffsetT {
	// We read the data as 5 little endian numbers, this will then be written out
	// as 5 little endian numbers into the byte stream. On the read side, we can
	// just copy the bytes out without needing to encode/decode.
	a := binary.LittleEndian.Uint32(hash[0:])
	b := binary.LittleEndian.Uint32(hash[4:])
	c := binary.LittleEndian.Uint32(hash[8:])
	d := binary.LittleEndian.Uint32(hash[12:])
	e := binary.LittleEndian.Uint32(hash[16:])
	return CreateGitHash(builder, a, b, c, d, e)
}

func sha256Write(builder *flatbuffers.Builder, hash *SHA256) flatbuffers.UOffsetT {
	if hash == nil {
		return 0
	}
	// We read the data as 8 little endian numbers, this will then be written out
	// as 8 little endian numbers into the byte stream. On the read side, we can
	// just copy the bytes out without needing to encode/decode.
	a := binary.LittleEndian.Uint32(hash[0:])
	b := binary.LittleEndian.Uint32(hash[4:])
	c := binary.LittleEndian.Uint32(hash[8:])
	d := binary.LittleEndian.Uint32(hash[12:])

	e := binary.LittleEndian.Uint32(hash[16:])
	f := binary.LittleEndian.Uint32(hash[20:])
	g := binary.LittleEndian.Uint32(hash[24:])
	h := binary.LittleEndian.Uint32(hash[28:])
	return CreateSha256(builder, a, b, c, d, e, f, g, h)
}

type docInnerW struct {
	hash        plumbing.Hash
	blockToLine []uint32
	data        []byte
}

func docInnerWrite(builder *flatbuffers.Builder, o docInnerW) flatbuffers.UOffsetT {
	data := builder.CreateByteVector(o.data)

	var blockToLine flatbuffers.UOffsetT
	if len(o.blockToLine) > 0 {
		blockToLine = uint32Vector(builder, o.blockToLine)
	}

	DocInnerStart(builder)
	DocInnerAddData(builder, data)
	DocInnerAddBlockToLine(builder, blockToLine)
	if !o.hash.IsZero() {
		DocInnerAddHash(builder, gitHashWrite(builder, o.hash))
	}
	return DocInnerEnd(builder)
}

type docW struct {
	path   flatbuffers.UOffsetT
	start  int64
	length int32
	size   int32
	modNs  int64
	typ    DocType
	sha256 *SHA256
}

func docWrite(builder *flatbuffers.Builder, o docW) flatbuffers.UOffsetT {
	DocStart(builder)
	DocAddSha256(builder, sha256Write(builder, o.sha256))
	DocAddType(builder, o.typ)
	DocAddModNs(builder, o.modNs)
	DocAddSize(builder, o.size)
	DocAddRange(builder, CreateByteRange(builder, o.start, o.length))
	DocAddPath(builder, o.path)
	return DocEnd(builder)
}

type postingListW struct {
	docs flatbuffers.UOffsetT
}

func postingListWrite(builder *flatbuffers.Builder, o postingListW) flatbuffers.UOffsetT {
	PostingListStart(builder)
	PostingListAddDocs(builder, o.docs)
	return PostingListEnd(builder)
}

type postingListsW struct {
	trigrams flatbuffers.UOffsetT
	lists    flatbuffers.UOffsetT
	maxDocID uint32
}

func postingListsWrite(builder *flatbuffers.Builder, o postingListsW) flatbuffers.UOffsetT {
	PostingListsStart(builder)
	PostingListsAddMaxDocId(builder, o.maxDocID)
	PostingListsAddLists(builder, o.lists)
	PostingListsAddTrigrams(builder, o.trigrams)
	return PostingListsEnd(builder)
}

type indexShardW struct {
	docs          flatbuffers.UOffsetT
	lists         flatbuffers.UOffsetT
	raw           flatbuffers.UOffsetT
	maxPathLength int32
}

func indexShardWrite(builder *flatbuffers.Builder, o indexShardW) flatbuffers.UOffsetT {
	IndexShardStart(builder)
	IndexShardAddMaxPathLength(builder, o.maxPathLength)
	IndexShardAddRaw(builder, o.raw)
	IndexShardAddLists(builder, o.lists)
	IndexShardAddDocs(builder, o.docs)
	return IndexShardEnd(builder)
}

func (i *IndexWriter) flushDocs(builder *flatbuffers.Builder, inlineRaw bool) (flatbuffers.UOffsetT, int32) {
	docs := make([]flatbuffers.UOffsetT, len(i.docs))
	var maxPathLength int
	for idx := len(i.docs) - 1; idx >= 0; idx-- {
		doc := i.docs[idx]

		var sha256 *SHA256
		if !inlineRaw {
			sha256 = &doc.ref.sha256
		}

		if maxPathLength < len(doc.path) {
			maxPathLength = len(doc.path)
		}

		docs[idx] = docWrite(builder, docW{
			path:   builder.CreateSharedString(doc.path),
			start:  doc.ref.start,
			length: doc.ref.length,
			size:   doc.size,
			modNs:  doc.mod.UnixNano(),
			typ:    doc.typ,
			sha256: sha256,
		})
	}

	return uOffsetTVector(builder, docs), int32(maxPathLength)
}

func (i *IndexWriter) flushPostingLists(builder *flatbuffers.Builder) (flatbuffers.UOffsetT, error) {
	trigramIdx := make([]int, len(i.denseTrigrams))
	for i := range trigramIdx {
		trigramIdx[i] = i
	}
	sort.Slice(trigramIdx, func(j, k int) bool {
		a := i.denseTrigrams[trigramIdx[j]]
		b := i.denseTrigrams[trigramIdx[k]]
		return a < b
	})

	tgms := make([]byte, 1+3*len(i.denseTrigrams))
	tgmsPos := len(tgms) - 4

	lists := make([]flatbuffers.UOffsetT, len(i.denseTrigrams))
	for idx := len(trigramIdx) - 1; idx >= 0; idx-- {
		binary.BigEndian.PutUint32(tgms[tgmsPos:], i.denseTrigrams[trigramIdx[idx]])
		tgmsPos -= 3

		bytes, err := i.lists[trigramIdx[idx]].ToBytes()
		if err != nil {
			return 0, err
		}
		lists[idx] = postingListWrite(builder, postingListW{
			docs: builder.CreateByteString(bytes),
		})
	}

	return postingListsWrite(builder, postingListsW{
		trigrams: builder.CreateByteVector(tgms[1:]),
		lists:    uOffsetTVector(builder, lists),
		maxDocID: uint32(len(i.docs)),
	}), nil
}

func (i *IndexWriter) ToBytes() ([]byte, error) {
	const maxInlineRaw = 16 << 20
	builder := flatbuffers.NewBuilder(2<<20 + maxInlineRaw)

	postingLists, err := i.flushPostingLists(builder)
	if err != nil {
		return nil, err
	}

	var raw flatbuffers.UOffsetT
	if i.rawLength < 16<<20 {
		flatRaw := make([]byte, 0, i.rawLength)
		for _, r := range i.raw {
			flatRaw = append(flatRaw, r...)
		}
		i.raw = nil
		raw = builder.CreateByteVector(flatRaw)
	}

	docs, maxPathLength := i.flushDocs(builder, raw != 0)
	builder.FinishWithFileIdentifier(indexShardWrite(builder, indexShardW{
		docs:          docs,
		lists:         postingLists,
		raw:           raw,
		maxPathLength: maxPathLength,
	}), []byte("IXS2"))

	return builder.FinishedBytes(), nil
}

// RawBytes provides access to the raw doc blob.
func (i *IndexWriter) RawBytes() [][]byte { return i.raw }
