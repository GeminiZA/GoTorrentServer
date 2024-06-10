package contentfile

type ContentFile struct {
	NumParts       int64
	PartCompletion []bool
	PartHashes     [][]byte
	Sha1Hash       []byte
}
