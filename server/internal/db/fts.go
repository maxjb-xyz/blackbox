package db

import "gorm.io/gorm"

const createEntriesFTSSQL = `
CREATE VIRTUAL TABLE IF NOT EXISTS entries_fts
USING fts5(
	id UNINDEXED,
	content,
	service,
	content='entries',
	content_rowid='rowid'
);
`

const createEntriesFTSInsertTrigger = `
CREATE TRIGGER IF NOT EXISTS entries_fts_insert
AFTER INSERT ON entries BEGIN
	INSERT INTO entries_fts(rowid, id, content, service)
	VALUES (new.rowid, new.id, new.content, new.service);
END;
`

const createEntriesFTSDeleteTrigger = `
CREATE TRIGGER IF NOT EXISTS entries_fts_delete
AFTER DELETE ON entries BEGIN
	INSERT INTO entries_fts(entries_fts, rowid, id, content, service)
	VALUES ('delete', old.rowid, old.id, old.content, old.service);
END;
`

const createEntriesFTSUpdateTrigger = `
CREATE TRIGGER IF NOT EXISTS entries_fts_update
AFTER UPDATE ON entries BEGIN
	INSERT INTO entries_fts(entries_fts, rowid, id, content, service)
	VALUES ('delete', old.rowid, old.id, old.content, old.service);
	INSERT INTO entries_fts(rowid, id, content, service)
	VALUES (new.rowid, new.id, new.content, new.service);
END;
`

func EnsureEntriesFTS(database *gorm.DB) error {
	for _, stmt := range []string{
		createEntriesFTSSQL,
		createEntriesFTSInsertTrigger,
		createEntriesFTSDeleteTrigger,
		createEntriesFTSUpdateTrigger,
	} {
		if err := database.Exec(stmt).Error; err != nil {
			return err
		}
	}

	var count int64
	if err := database.Table("entries_fts").Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		if err := database.Exec("INSERT INTO entries_fts(entries_fts) VALUES('rebuild')").Error; err != nil {
			return err
		}
	}
	return nil
}
