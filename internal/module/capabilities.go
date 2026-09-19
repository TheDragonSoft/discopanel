package module

import (
	"context"
	"sort"

	storage "github.com/nickheyer/discopanel/internal/db"
)

// CapabilityProviders returns a map of capability name -> the module on the
// given server that provides it. When several modules provide the same
// capability, the first one by creation time wins. Modules are templates'
// Provides values; excludeModuleID (typically the module being created or
// started) is skipped so a module never resolves against itself.
func CapabilityProviders(ctx context.Context, store *storage.Store, serverID string, excludeModuleID string) (map[string]*storage.Module, error) {
	modules, err := store.ListServerModules(ctx, serverID)
	if err != nil {
		return nil, err
	}

	// Sort by creation time so "first provider" is deterministic
	// (ListServerModules orders by name).
	ordered := make([]*storage.Module, len(modules))
	copy(ordered, modules)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].CreatedAt.Before(ordered[j].CreatedAt)
	})

	providers := make(map[string]*storage.Module)
	for _, m := range ordered {
		if m.ID == excludeModuleID {
			continue
		}
		template, err := store.GetModuleTemplate(ctx, m.TemplateID)
		if err != nil || template == nil || template.Provides == "" {
			continue
		}
		if _, exists := providers[template.Provides]; !exists {
			providers[template.Provides] = m
		}
	}

	return providers, nil
}

// FindCapabilityProvider returns the first module on the given server (by
// creation time) whose template provides the requested capability, or nil when
// none does. excludeModuleID is skipped (pass "" to exclude nothing).
func FindCapabilityProvider(ctx context.Context, store *storage.Store, serverID, capability, excludeModuleID string) (*storage.Module, error) {
	if capability == "" {
		return nil, nil
	}

	providers, err := CapabilityProviders(ctx, store, serverID, excludeModuleID)
	if err != nil {
		return nil, err
	}

	return providers[capability], nil
}
