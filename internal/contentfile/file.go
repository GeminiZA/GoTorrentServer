package contentfile

type ContentFile struct {
	NumParts uint32
	Parts    []Part
	Sha1Hash []byte
}

type Part struct {
	Complete bool
	Sha1Hash []byte
}