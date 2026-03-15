package db

// PlaylistAdapter combines *DB (read operations) and *Writer (write operations)
// to satisfy the playlistCreator and playlistRenamer interfaces defined in the
// TUI layer. Neither *DB nor *Writer alone satisfies those interfaces because
// reads live on *DB and writes live on *Writer.
type PlaylistAdapter struct {
	db     *DB
	writer *Writer
}

// NewPlaylistAdapter returns an adapter that forwards read operations to db
// and write operations to writer.
func NewPlaylistAdapter(db *DB, writer *Writer) *PlaylistAdapter {
	return &PlaylistAdapter{db: db, writer: writer}
}

// CreatePlaylist delegates to the writer goroutine.
func (a *PlaylistAdapter) CreatePlaylist(name string) (int64, error) {
	return a.writer.CreatePlaylist(name)
}

// RenamePlaylist delegates to the writer goroutine.
func (a *PlaylistAdapter) RenamePlaylist(id int64, name string) error {
	return a.writer.RenamePlaylist(id, name)
}

// ReadPlaylists delegates to the read-only DB connection.
func (a *PlaylistAdapter) ReadPlaylists() ([]PlaylistRow, error) {
	return a.db.ReadPlaylists()
}
