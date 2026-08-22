package mobility

// DefaultResolver keeps the local demo useful without a provider credential.
func DefaultResolver(provider Provider) *Resolver {
	return NewResolver(provider, map[string]StationMapping{
		"R04|R":       {StationID: "R04", LineID: "R", StationName: "信義安和"},
		"R03|R":       {StationID: "R03", LineID: "R", StationName: "台北101／世貿"},
		"R09|R":       {StationID: "R09", LineID: "R", StationName: "台大醫院"},
		"R10|R":       {StationID: "R10", LineID: "R", StationName: "台北車站"},
		"G12|G":       {StationID: "G12", LineID: "G", StationName: "西門"},
		"G07|G":       {StationID: "G07", LineID: "G", StationName: "公館"},
		"BL18|BL":     {StationID: "BL18", LineID: "BL", StationName: "市政府"},
		"BL12|BL":     {StationID: "BL12", LineID: "BL", StationName: "台北車站"},
		"BL07MALL|BL": {StationID: "BL07", LineID: "BL", StationName: "板橋"},
		"BL10MALL|BL": {StationID: "BL10", LineID: "BL", StationName: "龍山寺"},
		"O09|O":       {StationID: "O09", LineID: "O", StationName: "行天宮"},
	}, map[string]Context{
		"demo-beacon:1:4": {StationID: "R04", LineID: "R", PositionID: "exit-3", StationName: "信義安和", NearExit: true, Confidence: 0.75},
		"demo-beacon:1:3": {StationID: "R03", LineID: "R", PositionID: "platform", StationName: "台北101／世貿", Confidence: 0.75},
	}, Context{})
}
