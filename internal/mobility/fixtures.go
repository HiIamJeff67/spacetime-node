package mobility

// DefaultResolver keeps the local demo useful without a provider credential.
func DefaultResolver(provider Provider) *Resolver {
	return NewResolver(provider, map[string]StationMapping{
		"R|04":  {StationID: "R04", LineID: "R", StationName: "信義安和"},
		"R|03":  {StationID: "R03", LineID: "R", StationName: "台北101／世貿"},
		"BL|12": {StationID: "BL12", LineID: "BL", StationName: "市政府"},
	}, map[string]Context{
		"demo-beacon:1:4": {StationID: "R04", LineID: "R", PositionID: "exit-3", StationName: "信義安和", NearExit: true, Confidence: 0.75},
		"demo-beacon:1:3": {StationID: "R03", LineID: "R", PositionID: "platform", StationName: "台北101／世貿", Confidence: 0.75},
	}, Context{})
}
