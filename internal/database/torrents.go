package database

import "time"

type TargetFile struct {
	Name		string
	Path        string
	InfoHash    string
	Time        string
	Description string
	TrackerURL  string
	Complete	bool
	Parts		string
}

func (dbc *DBConn) AddTargetFile(tf TargetFile) error {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const insert string = `
	INSERT INTO torrents (info_hash, name, time, tracker_url, path, description, complete, parts)
	VALUES (?, ?, ?, ?, ?, ?, ?)
	`

	_, err := dbc.db.Exec(insert, tf.Name, tf.InfoHash, time.Now(), tf.TrackerURL, tf.Path, tf.Description, false, "")

	return err
}

func (dbc *DBConn) GetTargetFile(infoHash string) (*TargetFile, error) {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const query string = `
	SELECT info_hash, name, time, tracker_url, path, description
	FROM torrents
	WHERE info_hash = ?
	`
	var tf TargetFile
	err := dbc.db.QueryRow(query, infoHash).Scan(&tf.InfoHash, &tf.Name, &tf.Time, &tf.TrackerURL, &tf.Path, &tf.Description, &tf.Complete, &tf.Parts)

	if err != nil {
		return nil, err
	}

	return &tf, nil
}

func (dbc *DBConn) GetTargetFilePath(infoHash string) (string, error) {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const query string = `
	SELECT path
	FROM torrents
	WHERE info_hash = ?
	`

	var path string
	err := dbc.db.QueryRow(query, infoHash).Scan(&path)

	if err != nil {
		return "", err
	}

	return path, nil
}

func (dbc *DBConn) UpdateTargetFileTrackerURL(infoHash string, newTrackerUrl string) error {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const update string = `
	UPDATE torrents
	SET tracker_url = ?
	WHERE info_hash = ?
	`

	_, err := dbc.db.Exec(update, newTrackerUrl, infoHash)

	if err != nil {
		return err
	}

	return nil
}

func (dbc *DBConn) GetTargetFileTrackerURL(infoHash string) (string, error) {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const query string = `
	SELECT tracker_url
	FROM torrents
	WHERE info_hash = ?
	`

	var trackerUrl string
	err := dbc.db.QueryRow(query, infoHash).Scan(&trackerUrl)

	if err != nil {
		return "", err
	}

	return trackerUrl, nil
}

func (dbc *DBConn) GetTargetFileComplete(infoHash string) (bool, error) {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const query string = `
	SELECT complete FROM torrents
	where info_hash = ?
	`

	var complete bool
	err := dbc.db.QueryRow(query, infoHash).Scan(&complete)

	if err != nil {
		return false, err
	}

	return complete, nil
}

func (dbc *DBConn) GetTargetFileParts(infoHash string) (string, error) {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const query string = `
	SELECT parts FROM torrents
	where info_hash = ?
	`

	var parts string
	err := dbc.db.QueryRow(query, infoHash).Scan(&parts)

	if err != nil {
		return "", err
	}

	return parts, nil
}

func (dbc *DBConn) GetAllTargetFiles() ([]*TargetFile, error) {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const query string = `
	SELECT *
	FROM torrents
	`
	rows, err := dbc.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []*TargetFile
	for rows.Next() {
		var tf TargetFile
		if err := rows.Scan(&tf.InfoHash, &tf.Time, &tf.TrackerURL, &tf.Path, &tf.Description, &tf.Complete, &tf.Parts); err != nil {
			return nil, err
		}
		files = append(files, &tf)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return files, nil
}

func (dbc *DBConn) UpdateTargetFileParts(infoHash string, newParts string) error {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const update string = `
	UPDATE torrents
	SET parts = ?
	WHERE info_hash = ?
	`

	_, err := dbc.db.Exec(update, newParts, infoHash)

	if err != nil {
		return err
	}

	return nil
}

func (dbc *DBConn) UpdateTargetFileComplete(infoHash string, newComplete bool) error {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const update string = `
	UPDATE torrents
	SET complete = ?
	WHERE info_hash = ?
	`

	_, err := dbc.db.Exec(update, newComplete, infoHash)

	if err != nil {
		return err
	}

	return nil
}


func (dbc *DBConn) DeleteTargetFile(infoHash string) error {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const delete string = `
	DELETE FROM torrents
	WHERE info_hash = ?
	`

	_, err := dbc.db.Exec(delete, infoHash)

	return err
}