package terraria

import "github.com/nickheyer/discopanel/internal/db"

var SupportedVersions = map[db.TerrariaFlavor][]string{
	db.TerrariaFlavorVanilla: {
		"1.4.4.9",
		"1.4.4.x",
		"latest",
	},
	db.TerrariaFlavorTShock: {
		"v5.2",
		"latest",
	},
	db.TerrariaFlavorTModLoader: {
		"1.4",
		"1.3",
		"latest",
	},
}
