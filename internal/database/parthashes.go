package database

func (dbc *DBConn) insertPartHashes(infoHash string, parts []string) error {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const insertPart string = `
	INSERT INTO parts (info_hash, index, part_hash)
	VALUES (?, ?, ?)
	`
	for index, part := range parts {
		_, err := dbc.db.Exec(insertPart, infoHash, index, part)
		if err != nil {
			return err
		}
	}
	return nil
}

func (dbc *DBConn) getPartHash(infoHash string, index int) (string, error) {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const getPartQuery string = `
	SELECT part_hash FROM parts
	WHERE info_hash = ? AND index = ?
	`

	var partHash string
	err := dbc.db.QueryRow(getPartQuery, infoHash, index).Scan(&partHash)
	if err != nil {
		return "", err
	}
	return partHash, nil
}

func (dbc *DBConn) getAllPartHashes(infoHash string) ([]string, error) {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const getPartsQuery string = `
	SELECT part_hash FROM parts
	WHERE info_hash = ?
	ORDER BY index
	`

	rows, err := dbc.db.Query(getPartsQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var partHashes []string

	for rows.Next() {
		var partHash string
		if err := rows.Scan(&partHash); err != nil {
			return nil, err
		}
		partHashes = append(partHashes, partHash)
	}

	return partHashes, nil
}

func (dbc *DBConn) deletePartHashes(infoHash string) error {
	dbc.mu.Lock()
	defer dbc.mu.Unlock()

	const deleteParts string = `
	DELETE FROM parts
	WHERE info_hash = ?
	`

	_, err := dbc.db.Exec(deleteParts, infoHash)

	return err
}