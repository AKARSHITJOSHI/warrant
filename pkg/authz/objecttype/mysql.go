package authz

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/warrant-dev/warrant/pkg/database"
	"github.com/warrant-dev/warrant/pkg/middleware"
	"github.com/warrant-dev/warrant/pkg/service"
)

type MySQLRepository struct {
	database.SQLRepository
}

func NewMySQLRepository(db *database.MySQL) MySQLRepository {
	return MySQLRepository{
		database.NewSQLRepository(&db.SQL),
	}
}

func (repo MySQLRepository) Create(ctx context.Context, model Model) (int64, error) {
	result, err := repo.DB.ExecContext(
		ctx,
		`
			INSERT INTO objectType (
				typeId,
				definition
			) VALUES (?, ?)
			ON DUPLICATE KEY UPDATE
				definition = ?,
				createdAt = CURRENT_TIMESTAMP(6),
				deletedAt = NULL
		`,
		model.GetTypeId(),
		model.GetDefinition(),
		model.GetDefinition(),
	)
	if err != nil {
		return 0, errors.Wrap(err, "Unable to create object type")
	}

	newObjectTypeId, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	return newObjectTypeId, nil
}

func (repo MySQLRepository) GetById(ctx context.Context, id int64) (Model, error) {
	var objectType ObjectType
	err := repo.DB.GetContext(
		ctx,
		&objectType,
		`
			SELECT id, typeId, definition, createdAt, updatedAt, deletedAt
			FROM objectType
			WHERE
				id = ? AND
				deletedAt IS NULL
		`,
		id,
	)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return &objectType, service.NewRecordNotFoundError("ObjectType", id)
		default:
			return &objectType, err
		}
	}

	return &objectType, nil
}

func (repo MySQLRepository) GetByTypeId(ctx context.Context, typeId string) (Model, error) {
	var objectType ObjectType
	err := repo.DB.GetContext(
		ctx,
		&objectType,
		`
			SELECT id, typeId, definition, createdAt, updatedAt, deletedAt
			FROM objectType
			WHERE
				typeId = ? AND
				deletedAt IS NULL
		`,
		typeId,
	)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return &objectType, service.NewRecordNotFoundError("ObjectType", typeId)
		default:
			return &objectType, errors.Wrap(err, fmt.Sprintf("Unable to get ObjectType with typeId %s from mysql", typeId))
		}
	}

	return &objectType, nil
}

func (repo MySQLRepository) List(ctx context.Context, listParams middleware.ListParams) ([]Model, error) {
	models := make([]Model, 0)
	objectTypes := make([]ObjectType, 0)
	replacements := make([]interface{}, 0)
	query := `
		SELECT id, typeId, definition, createdAt, updatedAt, deletedAt
		FROM objectType
		WHERE
			deletedAt IS NULL
	`

	if listParams.Query != "" {
		searchTermReplacement := fmt.Sprintf("%%%s%%", listParams.Query)
		query = fmt.Sprintf("%s AND typeId LIKE ?", query)
		replacements = append(replacements, searchTermReplacement, searchTermReplacement)
	}

	if listParams.AfterId != "" {
		if listParams.AfterValue != nil {
			if listParams.SortOrder == middleware.SortOrderAsc {
				query = fmt.Sprintf("%s AND (%s > ? OR (typeId > ? AND %s = ?))", query, listParams.SortBy, listParams.SortBy)
				replacements = append(replacements,
					listParams.AfterValue,
					listParams.AfterId,
					listParams.AfterValue,
				)
			} else {
				query = fmt.Sprintf("%s AND (%s < ? OR (typeId < ? AND %s = ?))", query, listParams.SortBy, listParams.SortBy)
				replacements = append(replacements,
					listParams.AfterValue,
					listParams.AfterId,
					listParams.AfterValue,
				)
			}
		} else {
			if listParams.SortOrder == middleware.SortOrderAsc {
				query = fmt.Sprintf("%s AND typeId > ?", query)
				replacements = append(replacements, listParams.AfterId)
			} else {
				query = fmt.Sprintf("%s AND typeId < ?", query)
				replacements = append(replacements, listParams.AfterId)
			}
		}
	}

	if listParams.BeforeId != "" {
		if listParams.BeforeValue != nil {
			if listParams.SortOrder == middleware.SortOrderAsc {
				query = fmt.Sprintf("%s AND (%s < ? OR (typeId < ? AND %s = ?))", query, listParams.SortBy, listParams.SortBy)
				replacements = append(replacements,
					listParams.BeforeValue,
					listParams.BeforeId,
					listParams.BeforeValue,
				)
			} else {
				query = fmt.Sprintf("%s AND (%s > ? OR (typeId > ? AND %s = ?))", query, listParams.SortBy, listParams.SortBy)
				replacements = append(replacements,
					listParams.BeforeValue,
					listParams.BeforeId,
					listParams.BeforeValue,
				)
			}
		} else {
			if listParams.SortOrder == middleware.SortOrderAsc {
				query = fmt.Sprintf("%s AND typeId < ?", query)
				replacements = append(replacements, listParams.AfterId)
			} else {
				query = fmt.Sprintf("%s AND typeId > ?", query)
				replacements = append(replacements, listParams.AfterId)
			}
		}
	}

	if listParams.SortBy != "objectType" {
		query = fmt.Sprintf("%s ORDER BY %s %s, typeId %s LIMIT ?", query, listParams.SortBy, listParams.SortOrder, listParams.SortOrder)
		replacements = append(replacements, listParams.Limit)
	} else {
		query = fmt.Sprintf("%s ORDER BY typeId %s LIMIT ?", query, listParams.SortOrder)
		replacements = append(replacements, listParams.Limit)
	}

	err := repo.DB.SelectContext(
		ctx,
		&objectTypes,
		query,
		replacements...,
	)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return models, nil
		default:
			return models, errors.Wrap(err, "Unable to get object types from mysql")
		}
	}

	for i := range objectTypes {
		models = append(models, &objectTypes[i])
	}

	return models, nil
}

func (repo MySQLRepository) UpdateByTypeId(ctx context.Context, typeId string, model Model) error {
	_, err := repo.DB.ExecContext(
		ctx,
		`
			UPDATE objectType
			SET
				definition = ?
			WHERE
				typeId = ? AND
				deletedAt IS NULL
		`,
		model.GetDefinition(),
		typeId,
	)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Error updating object type %s", typeId))
	}

	return nil
}

func (repo MySQLRepository) DeleteByTypeId(ctx context.Context, typeId string) error {
	_, err := repo.DB.ExecContext(
		ctx,
		`
			UPDATE objectType
			SET
				deletedAt = ?
			WHERE
				typeId = ? AND
				deletedAt IS NULL
		`,
		time.Now().UTC(),
		typeId,
	)
	if err != nil {
		switch err {
		case sql.ErrNoRows:
			return service.NewRecordNotFoundError("ObjectType", typeId)
		default:
			return err
		}
	}

	return nil
}
