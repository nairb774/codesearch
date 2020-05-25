package index

import (
	"errors"
	"io/ioutil"

	"github.com/RoaringBitmap/roaring"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/google/codesearch/index"
)

var ErrInvalidIndex = errors.New("index invalid")

func Open(buff []byte) *IndexShard {
	return GetRootAsIndexShard(buff, 0)
}

func OpenFile(path string) (*IndexShard, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Open(b), nil
}

func (h *GitHash) PlumbingHash() (ret plumbing.Hash) {
	t := h.Table()
	copy(ret[:], t.Bytes[t.Pos:])
	return
}

func (p *PostingLists) FindTrigram(trigram uint32, bitmap *roaring.Bitmap) error {
	stride := 3
	tgms := p.TrigramsBytes()
	for lower, upper := 0, len(tgms)/stride; lower < upper; {
		// No overflow problems: only 1<<24 bits used for the trigrams.
		h := (lower + upper) >> 1
		idx := h * stride
		v := uint32(tgms[idx])<<16 | uint32(tgms[idx+1])<<8 | uint32(tgms[idx+2])
		if v == trigram {
			var pl PostingList
			if !p.Lists(&pl, h) {
				return ErrInvalidIndex
			}
			_, err := bitmap.FromBuffer(pl.DocsBytes())
			return err
		} else if v < trigram {
			lower = h + 1
		} else {
			upper = h
		}
	}

	return nil
}

func (g *Doc) PlumbingHash() (ret plumbing.Hash) {
	var h GitHash
	if g.Hash(&h) == nil {
		return
	}
	return h.PlumbingHash()
}

func (i *IndexShard) PostingQuery(q *index.Query) *roaring.Bitmap {
	return i.Lists(nil).PostingQuery(q, nil)
}

func (p *PostingLists) PostingQuery(q *index.Query, filter *roaring.Bitmap) *roaring.Bitmap {
	switch q.Op {
	case index.QNone:
		return roaring.New()

	case index.QAll:
		if filter == nil {
			filter := roaring.New()
			filter.Flip(0, uint64(p.MaxDocId()))
		}
		return filter

	case index.QAnd:
		for _, t := range q.Trigram {
			l := roaring.New()
			if err := p.FindTrigram(uint32(t[0])<<16|uint32(t[1])<<8|uint32(t[2]), l); err != nil {
				panic(err)
			}
			if filter != nil {
				l.And(filter)
			}
			filter = l
			if filter.IsEmpty() {
				return filter
			}
		}

		for _, s := range q.Sub {
			filter = p.PostingQuery(s, filter)
			if filter.IsEmpty() {
				return filter
			}
		}
		return filter

	case index.QOr:
		if filter == nil {
			var list *roaring.Bitmap
			for _, s := range q.Sub {
				l := p.PostingQuery(s, nil)
				if list == nil {
					list = l
				} else {
					list.Or(l)
				}
			}

			for _, t := range q.Trigram {
				l := roaring.New()
				if err := p.FindTrigram(uint32(t[0])<<16|uint32(t[1])<<8|uint32(t[2]), l); err != nil {
					panic(err)
				}
				if list == nil {
					list = l
				} else {
					list.Or(l)
				}
			}
			return list
		}

		notFound := filter.Clone()
		for _, s := range q.Sub {
			l := p.PostingQuery(s, filter)
			notFound.AndNot(l)
			if notFound.IsEmpty() {
				return filter
			}
		}

		for _, t := range q.Trigram {
			l := roaring.New()
			if err := p.FindTrigram(uint32(t[0])<<16|uint32(t[1])<<8|uint32(t[2]), l); err != nil {
				panic(err)
			}
			notFound.AndNot(l)
			if notFound.IsEmpty() {
				return filter
			}
		}
		notFound.Xor(filter)
		return notFound
	}
	panic(q.Op)
}
