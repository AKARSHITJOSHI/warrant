package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/ngrok/sqlmw"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
)

// NullString type representing a nullable string
type NullString struct {
	sql.NullString
}

// MarshalJSON returns the marshaled json string
func (s NullString) MarshalJSON() ([]byte, error) {
	if s.Valid {
		return json.Marshal(s.String)
	}

	return []byte(`null`), nil
}

// UnmarshalJSON returns the unmarshaled struct
func (s *NullString) UnmarshalJSON(data []byte) error {
	var str *string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	if str != nil {
		s.Valid = true
		s.String = *str
	} else {
		s.Valid = false
	}

	return nil
}

func StringToNullString(str *string) NullString {
	if str == nil {
		return NullString{
			sql.NullString{},
		}
	}

	return NullString{
		sql.NullString{
			Valid:  true,
			String: *str,
		},
	}
}

// NullTime type representing a nullable string
type NullTime struct {
	sql.NullTime
}

// MarshalJSON returns the marshaled json string
func (t NullTime) MarshalJSON() ([]byte, error) {
	if t.Valid {
		return json.Marshal(t.Time)
	}

	return []byte(`null`), nil
}

type SqlQueryable interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
	NamedExecContext(ctx context.Context, query string, arg interface{}) (sql.Result, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
	SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error
}

type SqlTx struct {
	Tx *sqlx.Tx
}

func (q SqlTx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	result, err := q.Tx.Exec(query, args...)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return result, err
		default:
			return result, errors.Wrap(err, "sql error")
		}
	}
	return result, err
}

func (q SqlTx) GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	err := q.Tx.Get(dest, query, args...)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return err
		default:
			return errors.Wrap(err, "sql error")
		}
	}
	return err
}

func (q SqlTx) NamedExecContext(ctx context.Context, query string, arg interface{}) (sql.Result, error) {
	result, err := q.Tx.NamedExecContext(ctx, query, arg)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return result, err
		default:
			return result, errors.Wrap(err, "sql error")
		}
	}
	return result, err
}

func (q SqlTx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	stmt, err := q.Tx.PrepareContext(ctx, query)
	if err != nil {
		return stmt, errors.Wrap(err, "sql error")
	}
	return stmt, err
}

func (q SqlTx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	rows, err := q.Tx.QueryContext(ctx, query, args...)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return rows, err
		default:
			return rows, errors.Wrap(err, "sql error")
		}
	}
	return rows, err
}

func (q SqlTx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return q.Tx.QueryRowContext(ctx, query, args...)
}

func (q SqlTx) SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	err := q.Tx.SelectContext(ctx, dest, query, args...)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return err
		default:
			return errors.Wrap(err, "sql error")
		}
	}
	return err
}

type SQL struct {
	DB *sqlx.DB
}

func (ds SQL) WithinTransaction(ctx context.Context, txFunc func(ctx context.Context) error) error {
	// If transaction already started, re-use it
	if _, ok := ctx.Value(txKey{}).(*SqlTx); ok {
		err := txFunc(ctx)
		return err
	}

	tx, err := ds.DB.Beginx()
	if err != nil {
		return errors.Wrap(err, "Error beginning sql transaction")
	}

	defer func() {
		if p := recover(); p != nil {
			err = tx.Rollback()
			if err != nil {
				log.Err(err).Msg("error rolling back sql transaction")
			}

			panic(p)
		} else if err != nil {
			err = tx.Rollback()
			if err != nil {
				log.Err(err).Msg("error rolling back sql transaction")
			}
		} else {
			err = tx.Commit()
			if err != nil {
				log.Err(err).Msg("error committing sql transaction")
			}
		}
	}()

	err = txFunc(context.WithValue(ctx, txKey{}, &SqlTx{
		Tx: tx,
	}))
	return err
}

func (ds SQL) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	queryable := ds.getQueryableFromContext(ctx)
	result, err := queryable.ExecContext(ctx, query, args...)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return result, err
		default:
			return result, errors.Wrap(err, "Error when calling mysql ExecContext")
		}
	}
	return result, err
}

func (ds SQL) GetContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	queryable := ds.getQueryableFromContext(ctx)
	err := queryable.GetContext(ctx, dest, query, args...)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return err
		default:
			return errors.Wrap(err, "Error when calling mysql GetContext")
		}
	}
	return err
}

func (ds SQL) NamedExecContext(ctx context.Context, query string, arg interface{}) (sql.Result, error) {
	queryable := ds.getQueryableFromContext(ctx)
	result, err := queryable.NamedExecContext(ctx, query, arg)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return result, err
		default:
			return result, errors.Wrap(err, "Error when calling mysql NamedExecContext")
		}
	}
	return result, err
}

func (ds SQL) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	queryable := ds.getQueryableFromContext(ctx)
	stmt, err := queryable.PrepareContext(ctx, query)
	if err != nil {
		return stmt, errors.Wrap(err, "Error when calling mysql PrepareContext")
	}
	return stmt, err
}

func (ds SQL) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	queryable := ds.getQueryableFromContext(ctx)
	rows, err := queryable.QueryContext(ctx, query, args...)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return rows, err
		default:
			return rows, errors.Wrap(err, "Error when calling mysql QueryContext")
		}
	}
	return rows, err
}

func (ds SQL) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	queryable := ds.getQueryableFromContext(ctx)
	return queryable.QueryRowContext(ctx, query, args...)
}

func (ds SQL) SelectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	queryable := ds.getQueryableFromContext(ctx)
	err := queryable.SelectContext(ctx, dest, query, args...)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return err
		default:
			return errors.Wrap(err, "Error when calling mysql SelectContext")
		}
	}
	return err
}

func (ds SQL) getQueryableFromContext(ctx context.Context) SqlQueryable {
	if tx, ok := ctx.Value(txKey{}).(*SqlTx); ok {
		return tx
	} else {
		return ds.DB
	}
}

// SQLRepository type
type SQLRepository struct {
	DB *SQL
}

// NewSQLRepository returns an instance of SQLRepository
func NewSQLRepository(db *SQL) SQLRepository {
	if db == nil {
		log.Fatal().Msg("Cannot initialize SQLRepository with a nil db parameter")
	}

	return SQLRepository{
		DB: db,
	}
}

// SQLInterceptor type
type SQLInterceptor struct {
	sqlmw.NullInterceptor
}

// StmtQueryContext overrides the base StmtQueryContext sql method and adds latency measurement and logging
func (in *SQLInterceptor) StmtQueryContext(ctx context.Context, conn driver.StmtQueryContext, query string, args []driver.NamedValue) (context.Context, driver.Rows, error) {
	startedAt := time.Now()
	rows, err := conn.QueryContext(ctx, args)
	duration := time.Since(startedAt)
	if duration.Milliseconds() > 50 {
		log.Warn().
			Str("query", strings.Join(strings.Fields(query), " ")).
			Str("args", fmt.Sprintf("%v", args)).
			Err(err).
			Dur("duration", duration).
			Msg("Slow SQL query")
	}
	return ctx, rows, err
}

// StmtExecContext overrides the base StmtExecContext sql method and adds latency measurement and logging
func (in *SQLInterceptor) StmtExecContext(ctx context.Context, conn driver.StmtExecContext, query string, args []driver.NamedValue) (driver.Result, error) {
	startedAt := time.Now()
	result, err := conn.ExecContext(ctx, args)
	duration := time.Since(startedAt)
	if duration.Milliseconds() > 50 {
		log.Warn().
			Str("query", strings.Join(strings.Fields(query), " ")).
			Str("args", fmt.Sprintf("%v", args)).
			Err(err).
			Dur("duration", duration).
			Msg("Slow SQL query")
	}
	return result, err
}
