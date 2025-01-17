package authz

import (
	"context"

	object "github.com/warrant-dev/warrant/pkg/authz/object"
	objecttype "github.com/warrant-dev/warrant/pkg/authz/objecttype"
	"github.com/warrant-dev/warrant/pkg/event"
	"github.com/warrant-dev/warrant/pkg/middleware"
	"github.com/warrant-dev/warrant/pkg/service"
)

const ResourceTypeRole = "role"

type RoleService struct {
	service.BaseService
	repo      RoleRepository
	eventSvc  event.EventService
	objectSvc object.ObjectService
}

func NewService(env service.Env, repo RoleRepository, eventSvc event.EventService, objectSvc object.ObjectService) RoleService {
	return RoleService{
		BaseService: service.NewBaseService(env),
		repo:        repo,
		eventSvc:    eventSvc,
		objectSvc:   objectSvc,
	}
}

func (svc RoleService) Create(ctx context.Context, roleSpec RoleSpec) (*RoleSpec, error) {
	var newRole Model
	err := svc.Env().DB().WithinTransaction(ctx, func(txCtx context.Context) error {
		createdObject, err := svc.objectSvc.Create(txCtx, *roleSpec.ToObjectSpec())
		if err != nil {
			return err
		}

		_, err = svc.repo.GetByRoleId(txCtx, roleSpec.RoleId)
		if err == nil {
			return service.NewDuplicateRecordError("Role", roleSpec.RoleId, "A role with the given roleId already exists")
		}

		newRoleId, err := svc.repo.Create(txCtx, roleSpec.ToRole(createdObject.ID))
		if err != nil {
			return err
		}

		newRole, err = svc.repo.GetById(txCtx, newRoleId)
		if err != nil {
			return err
		}

		svc.eventSvc.TrackResourceCreated(txCtx, ResourceTypeRole, newRole.GetRoleId(), newRole.ToRoleSpec())
		return nil
	})

	if err != nil {
		return nil, err
	}

	return newRole.ToRoleSpec(), nil
}

func (svc RoleService) GetByRoleId(ctx context.Context, roleId string) (*RoleSpec, error) {
	roleRepository, err := NewRepository(svc.Env().DB())
	if err != nil {
		return nil, err
	}

	role, err := roleRepository.GetByRoleId(ctx, roleId)
	if err != nil {
		return nil, err
	}

	return role.ToRoleSpec(), nil
}

func (svc RoleService) List(ctx context.Context, listParams middleware.ListParams) ([]RoleSpec, error) {
	roleSpecs := make([]RoleSpec, 0)
	roleRepository, err := NewRepository(svc.Env().DB())
	if err != nil {
		return roleSpecs, err
	}

	roles, err := roleRepository.List(ctx, listParams)
	if err != nil {
		return roleSpecs, nil
	}

	for _, role := range roles {
		roleSpecs = append(roleSpecs, *role.ToRoleSpec())
	}

	return roleSpecs, nil
}

func (svc RoleService) UpdateByRoleId(ctx context.Context, roleId string, roleSpec UpdateRoleSpec) (*RoleSpec, error) {
	roleRepository, err := NewRepository(svc.Env().DB())
	if err != nil {
		return nil, err
	}

	currentRole, err := roleRepository.GetByRoleId(ctx, roleId)
	if err != nil {
		return nil, err
	}

	currentRole.SetName(roleSpec.Name)
	currentRole.SetDescription(roleSpec.Description)
	err = roleRepository.UpdateByRoleId(ctx, roleId, currentRole)
	if err != nil {
		return nil, err
	}

	updatedRole, err := roleRepository.GetByRoleId(ctx, roleId)
	if err != nil {
		return nil, err
	}

	updatedRoleSpec := updatedRole.ToRoleSpec()
	svc.eventSvc.TrackResourceUpdated(ctx, ResourceTypeRole, updatedRole.GetRoleId(), updatedRoleSpec)
	return updatedRoleSpec, nil
}

func (svc RoleService) DeleteByRoleId(ctx context.Context, roleId string) error {
	err := svc.Env().DB().WithinTransaction(ctx, func(txCtx context.Context) error {
		roleRepository, err := NewRepository(svc.Env().DB())
		if err != nil {
			return err
		}

		err = roleRepository.DeleteByRoleId(txCtx, roleId)
		if err != nil {
			return err
		}

		err = svc.objectSvc.DeleteByObjectTypeAndId(txCtx, objecttype.ObjectTypeRole, roleId)
		if err != nil {
			return err
		}

		svc.eventSvc.TrackResourceDeleted(txCtx, ResourceTypeRole, roleId, nil)
		return nil
	})

	return err
}
