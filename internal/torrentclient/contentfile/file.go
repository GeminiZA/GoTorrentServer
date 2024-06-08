package contentfile

type ContentFile struct {
	NumParts       uint32
	PartCompletion []bool
	PartHashes     [][]byte
	Sha1Hash       []byte
}
