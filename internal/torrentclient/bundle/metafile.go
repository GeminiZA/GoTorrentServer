package bundle

import (
	"github.com/GeminiZA/GoTorrentServer/internal/torrentclient/torrentfile"
)

type MetaFile struct {
	MultiFile bool
	Files []torrentfile.FileInfo
	Length int64
	Data     *torrentfile.TorrentFile
}

func CreateMetaFile(tf *torrentfile.TorrentFile) (*MetaFile, error) {
	mf := MetaFile{Data: tf}
	if tf.Info.Length != 0 {
		mf.MultiFile = true
		mf.Files = tf.Info.Files
		tot := 0
		for i := range mf.Files {
			tot += int(mf.Files[i].Length)
		}
		mf.Length = int64(tot)
	} else {
		mf.MultiFile = false
		mf.Length = tf.Info.Length
	}
	return nil, nil
}

func LoadMetaFile(path string) (*MetaFile, error) {

	return nil, nil
}
