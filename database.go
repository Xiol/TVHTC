package main

import (
	"database/sql"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"time"
)

type Database struct {
	db   *sql.DB
	path string
}

func NewDatabase(path string) *Database {
	return &Database{path: path}
}

// Connect to the database and actually Ping() it to ensure our
// connection is good.
func (this *Database) Open() {
	var err error
	this.db, err = sql.Open("sqlite3", this.path)
	if err != nil {
		Log.Fatalf("Error opening database: %v", err)
	}
	err = this.db.Ping()
	if err != nil {
		Log.Fatalf("Could not ping database after successful open: %v", err)
	}
	return
}

func (this *Database) Close() {
	this.db.Close()
}

// Create our schema if necessary. This function will open the connection to
// the database if it's not already open.
func (this *Database) Initialise() {
	// One single giant table because normalisation is for jerks who have
	// a lot of time on their hands and serious things to do.
	create_stmt := `CREATE TABLE IF NOT EXISTS transcodes (
					id INTEGER NOT NULL PRIMARY KEY, path TEXT, 
					filename TEXT, channel TEXT, title TEXT, 
					status TEXT, completed INTEGER, message TEXT,
					elapsedtime INTEGER, initialqueuetime DATETIME, 
					completetime DATETIME, sizebefore INTEGER, sizeafter INTEGER);`

	if this.db == nil {
		this.Open()
	}
	if _, err := this.db.Exec(create_stmt); err != nil {
		Log.Fatalf("Could not create database table: %v", err)
	}

	return
}

// Add a new job to the database.
func (this *Database) AddEntry(t *TVHJob) (int64, error) {
	Log.Debug("Adding database entry for job: %+v", t)
	stmt, err := this.db.Prepare(`INSERT INTO transcodes (path, filename, channel, title, status, 
								  completed, message, elapsedtime, initialqueuetime, completetime, 
								  sizebefore, sizeafter) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);`)
	if err != nil {
		return -1, fmt.Errorf("Error creating prepared statement: %v", err)
	}
	defer stmt.Close()

	res, err := stmt.Exec(t.Path, t.Filename, t.Channel, t.Title, t.Status, false, "", 0, time.Now(), nil, 0, 0)
	if err != nil {
		return -1, fmt.Errorf("Could not add job to database: %v", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return -1, fmt.Errorf("Unable to get last insert ID: %v", err)
	}

	return id, nil
}

// Mark a job as completed.
func (this *Database) Complete(t *TranscodeJob) error {
	Log.Debug("Completing database entry for job: %+v", t.Job)
	stmt, err := this.db.Prepare(`UPDATE transcodes SET completed=?, message=?, elapsedtime=?, completetime=?,
								  sizebefore=?, sizeafter=? WHERE id=?`)
	if err != nil {
		return fmt.Errorf("Error creating prepared statement: %v", err)
	}

	_, err = stmt.Exec(true, t.Message, t.ElapsedTime.Nanoseconds(), time.Now(), t.OldSize, t.NewSize, t.Job.DBID)
	if err != nil {
		return fmt.Errorf("Error completing job: %v", err)
	}

	return nil
}

// Recover any uncompleted jobs from the database
// and add them back onto the transcode queue
func (this *Database) Recover() error {
	Log.Debug("Doing job recovery...")

	rows, err := this.db.Query("SELECT id, path, filename, channel, title, status FROM transcodes WHERE completed=0")
	if err != nil {
		return fmt.Errorf("Error querying database: %v", err)
	}
	defer rows.Close()

	i := 0
	for rows.Next() {
		job := &TVHJob{}
		//err := rows.Scan(&path, &filename, &channel, &title, &status)
		err := rows.Scan(&job.DBID, &job.Path, &job.Filename, &job.Channel, &job.Title, &job.Status)
		if err != nil {
			return fmt.Errorf("Error retrieving row from database: %v", err)
		}
		Transcode(job)
		i++
	}
	err = rows.Err()
	if err != nil {
		return fmt.Errorf("Error during recovery: %v", err)
	}

	Log.Warning("Recovered %v incomplete jobs", i)

	return nil
}

func (this *Database) IncompleteJobs() ([]TVHJob, error) {
	Log.Debug("Getting incomplete job list...")
	jobs := make([]TVHJob, 0)

	rows, err := this.db.Query("SELECT id, path, filename, channel, title, status FROM transcodes WHERE completed=0")
	if err != nil {
		return nil, fmt.Errorf("Error querying database: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		job := TVHJob{}
		err := rows.Scan(&job.DBID, &job.Path, &job.Filename, &job.Channel, &job.Title, &job.Status)
		if err != nil {
			return nil, fmt.Errorf("Error retrieving row from database: %v", err)
		}
		jobs = append(jobs, job)
	}
	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("Error retrieving incomplete jobs: %v", err)
	}
	return jobs, nil
}
