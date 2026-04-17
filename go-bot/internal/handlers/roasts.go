package handlers

// usernameToName maps Telegram usernames to the name keys used in DB.
var usernameToName = map[string]string{
	"kondzhariia_data": "Data",
	"kondzhariia":      "Data",
	"Dany_ro":          "Danya",
	"facethestrange":   "Bo",
}

func resolveTarget(name, username string) string {
	if mapped, ok := usernameToName[username]; ok {
		return mapped
	}
	return name
}
