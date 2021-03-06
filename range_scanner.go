package recordio

import "io"

// Index consists offsets and sizes of the consequetive chunks in a RecordIO file.
//
// Index supports Gob. Every field in the Index needs to be exported
// for the correct encoding and decoding using Gob.
type Index struct {
	ChunkOffsets []int64
	ChunkLens    []uint32
	NumRecords   int   // the number of all records in a file.
	ChunkRecords []int // the number of records in chunks.
}

// LoadIndex scans the file and parse chunkOffsets, chunkLens, and len.
func LoadIndex(r io.ReadSeeker) (*Index, error) {
	offset, e := r.Seek(0, io.SeekCurrent)
	if e != nil {
		return nil, e
	}

	f := &Index{}
	var hdr *Header

	for {
		hdr, e = parseHeader(r)
		if e != nil {
			break
		}

		f.ChunkOffsets = append(f.ChunkOffsets, offset)
		f.ChunkLens = append(f.ChunkLens, hdr.numRecords)
		f.ChunkRecords = append(f.ChunkRecords, int(hdr.numRecords))
		f.NumRecords += int(hdr.numRecords)

		offset, e = r.Seek(int64(hdr.compressedSize), io.SeekCurrent)
		if e != nil {
			break
		}
	}

	if e == io.EOF {
		return f, nil
	}
	return nil, e
}

// NumChunks returns the total number of chunks in a RecordIO file.
func (r *Index) NumChunks() int {
	return len(r.ChunkLens)
}

// ChunkIndex return the Index of i-th Chunk.
func (r *Index) ChunkIndex(i int) *Index {
	idx := &Index{}
	idx.ChunkOffsets = []int64{r.ChunkOffsets[i]}
	idx.ChunkLens = []uint32{r.ChunkLens[i]}
	idx.ChunkRecords = []int{r.ChunkRecords[i]}
	idx.NumRecords = idx.ChunkRecords[0]
	return idx
}

// Locate returns the index of chunk that contains the given record,
// and the record index within the chunk.  It returns (-1, -1) if the
// record is out of range.
func (r *Index) Locate(recordIndex int) (int, int) {
	sum := 0
	for i, l := range r.ChunkLens {
		sum += int(l)
		if recordIndex < sum {
			return i, recordIndex - sum + int(l)
		}
	}
	return -1, -1
}

// RangeScanner scans records in a specified range within [0, numRecords).
type RangeScanner struct {
	reader          io.ReadSeeker
	index           *Index
	start, end, cur int
	chunkIndex      int
	chunk           *Chunk
	err             error
}

// NewRangeScanner creates a scanner that sequencially reads records in the
// range [start, start+len).  If start < 0, it scans from the
// beginning.  If len < 0, it scans till the end of file.
func NewRangeScanner(r io.ReadSeeker, index *Index, start, len int) *RangeScanner {
	if start < 0 {
		start = 0
	}
	if len < 0 || start+len >= index.NumRecords {
		len = index.NumRecords - start
	}

	return &RangeScanner{
		reader:     r,
		index:      index,
		start:      start,
		end:        start + len,
		cur:        start - 1, // The intial status required by Scan.
		chunkIndex: -1,
		chunk:      &Chunk{},
	}
}

// Scan moves the cursor forward for one record and loads the chunk
// containing the record if not yet.
func (s *RangeScanner) Scan() bool {
	s.cur++

	if s.cur >= s.end {
		s.err = io.EOF
	} else {
		if ci, _ := s.index.Locate(s.cur); s.chunkIndex != ci {
			s.chunkIndex = ci
			s.chunk, s.err = parseChunk(s.reader, s.index.ChunkOffsets[ci])
		}
	}

	return s.err == nil
}

// Record returns the record under the current cursor.
func (s *RangeScanner) Record() []byte {
	_, ri := s.index.Locate(s.cur)
	return s.chunk.records[ri]
}

// Err returns the first non-EOF error that was encountered by the
// Scanner.
func (s *RangeScanner) Err() error {
	if s.err == io.EOF {
		return nil
	}

	return s.err
}
