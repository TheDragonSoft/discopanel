package services

import (
	"context"

	"connectrpc.com/connect"
	"github.com/nickheyer/discopanel/internal/db"
	"github.com/nickheyer/discopanel/internal/terraria"
	v1 "github.com/nickheyer/discopanel/pkg/proto/discopanel/v1"
	"github.com/nickheyer/discopanel/pkg/proto/discopanel/v1/discopanelv1connect"
)

type TerrariaServiceHandler struct {
	store *db.Store
}

func NewTerrariaServiceHandler(store *db.Store) *TerrariaServiceHandler {
	return &TerrariaServiceHandler{store: store}
}

func (h *TerrariaServiceHandler) GetTerrariaVersions(ctx context.Context, req *connect.Request[v1.GetTerrariaVersionsRequest]) (*connect.Response[v1.GetTerrariaVersionsResponse], error) {
	flavor := db.TerrariaFlavorVanilla
	switch req.Msg.Flavor {
	case v1.TerrariaFlavor_TERRARIA_FLAVOR_TSHOCK:
		flavor = db.TerrariaFlavorTShock
	case v1.TerrariaFlavor_TERRARIA_FLAVOR_TMODLOADER:
		flavor = db.TerrariaFlavorTModLoader
	}

	versions := terraria.SupportedVersions[flavor]
	return connect.NewResponse(&v1.GetTerrariaVersionsResponse{
		Versions: versions,
	}), nil
}

func (h *TerrariaServiceHandler) GetTerrariaFlavors(ctx context.Context, req *connect.Request[v1.GetTerrariaFlavorsRequest]) (*connect.Response[v1.GetTerrariaFlavorsResponse], error) {
	return connect.NewResponse(&v1.GetTerrariaFlavorsResponse{
		Flavors: []v1.TerrariaFlavor{
			v1.TerrariaFlavor_TERRARIA_FLAVOR_VANILLA,
			v1.TerrariaFlavor_TERRARIA_FLAVOR_TSHOCK,
			v1.TerrariaFlavor_TERRARIA_FLAVOR_TMODLOADER,
		},
	}), nil
}

func (h *TerrariaServiceHandler) GetTerrariaConfig(ctx context.Context, req *connect.Request[v1.GetTerrariaConfigRequest]) (*connect.Response[v1.GetTerrariaConfigResponse], error) {
	config, err := h.store.GetTerrariaConfig(ctx, req.Msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	return connect.NewResponse(&v1.GetTerrariaConfigResponse{
		Config: &v1.TerrariaConfig{
			WorldName:       config.WorldName,
			WorldSize:       config.WorldSize,
			Difficulty:      int32(config.Difficulty),
			Seed:            config.Seed,
			Password:        config.Password,
			MaxPlayers:      int32(config.MaxPlayers),
			Motd:            config.MOTD,
			CustomConfig:    config.CustomConfig,
			BanListPath:     config.BanListPath,
			SpawnProtection: config.SpawnProtection,
			Secure:          config.Secure,
			Language:        config.Language,
		},
	}), nil
}

func (h *TerrariaServiceHandler) UpdateTerrariaConfig(ctx context.Context, req *connect.Request[v1.UpdateTerrariaConfigRequest]) (*connect.Response[v1.UpdateTerrariaConfigResponse], error) {
	config, err := h.store.GetTerrariaConfig(ctx, req.Msg.ServerId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	msgConf := req.Msg.Config
	config.WorldName = msgConf.WorldName
	config.WorldSize = msgConf.WorldSize
	config.Difficulty = int(msgConf.Difficulty)
	config.Seed = msgConf.Seed
	config.Password = msgConf.Password
	config.MaxPlayers = int(msgConf.MaxPlayers)
	config.MOTD = msgConf.Motd
	config.CustomConfig = msgConf.CustomConfig
	config.BanListPath = msgConf.BanListPath
	config.SpawnProtection = msgConf.SpawnProtection
	config.Secure = msgConf.Secure
	config.Language = msgConf.Language

	if err := h.store.UpdateTerrariaConfig(ctx, config); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&v1.UpdateTerrariaConfigResponse{
		Config: msgConf,
	}), nil
}

// Interface check
var _ discopanelv1connect.TerrariaServiceHandler = (*TerrariaServiceHandler)(nil)
