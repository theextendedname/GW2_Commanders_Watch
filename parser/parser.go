package parser

import (
	"encoding/json"
	"os"
	"strconv"
)

// FlexFloat64 is a custom type to handle JSON values that can be either a float or a string.
type FlexFloat64 float64

// UnmarshalJSON implements the json.Unmarshaler interface.
func (f *FlexFloat64) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a float64 first.
	var floatVal float64
	if err := json.Unmarshal(data, &floatVal); err == nil {
		*f = FlexFloat64(floatVal)
		return nil
	}

	// If that fails, try to unmarshal as a string.
	var stringVal string
	if err := json.Unmarshal(data, &stringVal); err != nil {
		return err // It's neither a float nor a string that we can handle.
	}

	// If the string can be parsed to a float, use that value.
	if val, err := strconv.ParseFloat(stringVal, 64); err == nil {
		*f = FlexFloat64(val)
		return nil
	}

	// If the string is not a valid float (e.g., "N/A"), default to 0.
	*f = 0.0
	return nil
}

// This file contains the Go structs for parsing the JSON output from Guild Wars 2 Elite Insights.
// The structure is based on the format for `_detailed_wvw_kill.json` files.

type ParsedLog struct {
	FightName            string               `json:"fightName"`
	TimeStart            string               `json:"timeStart"`
	Duration             string               `json:"duration"`
	EncounterDuration    string               `json:"encounterDuration"`
	Players              []Player             `json:"players"`
	Targets              []Target             `json:"targets"`
	Mechanics            []Mechanic           `json:"mechanics"`
	CombatReplayMetaData CombatReplayMetaData `json:"combatReplayMetaData"`
}

type Player struct {
	Name             string               `json:"name"`
	Account          string               `json:"account"`
	Profession       string               `json:"profession"`
	HasCommanderTag  bool                 `json:"hasCommanderTag"`
	NotInSquad       bool                 `json:"notInSquad"`
	StatsAll         []PlayerStats        `json:"statsAll"`
	DpsAll           []PlayerDps          `json:"dpsAll"`
	DpsTargets       [][]PlayerTargetDps  `json:"dpsTargets"`
	Defenses         []PlayerDefense      `json:"defenses"`
	Support          []PlayerSupport      `json:"support"`
	StatsTargets     [][]PlayerStatTarget `json:"statsTargets"`
	CombatReplayData CombatReplayData     `json:"combatReplayData"`
	ExtHealingStats  ExtHealingStats      `json:"extHealingStats"`
	ExtBarrierStats  ExtBarrierStats      `json:"extBarrierStats"`
}

type PlayerDps struct {
	Dps int `json:"dps"`
}
type PlayerTargetDps struct {
	Dps    int `json:"dps"`
	Damage int `json:"damage"`
}
type PlayerStats struct {
	Dmg              int         `json:"totaldmg"`
	Downed           int         `json:"downed"`
	Killed           int         `json:"killed"`
	DownContribution int         `json:"downContribution"`
	DistToCommander  FlexFloat64 `json:"distToCom"`
}

type PlayerStatTarget struct {
	Downed           int `json:"downed"`
	Killed           int `json:"killed"`
	DownContribution int `json:"downContribution"`
}

type PlayerDefense struct {
	DownCount            int `json:"downCount"`
	DeadCount            int `json:"deadCount"`
	ReceivedCrowdControl int `json:"receivedCrowdControl"`
}

type PlayerSupport struct {
	BoonStrips       int `json:"boonStrips"`
	CondiCleanse     int `json:"condiCleanse"`
	CondiCleanseSelf int `json:"condiCleanseSelf"`
}

type CombatReplayData struct {
	Down      []interface{}   `json:"down"`
	Dead      [][]interface{} `json:"dead"`
	Positions [][]float64     `json:"positions"`
}

type CombatReplayMetaData struct {
	PollingRate int `json:"pollingRate"`
}

type ExtHealingStats struct {
	OutgoingHealingAllies [][]Healing `json:"outgoingHealingAllies"`
}

type Healing struct {
	Healing int `json:"healing"`
	Hps     int `json:"hps"`
}

type ExtBarrierStats struct {
	OutgoingBarrier []Barrier `json:"outgoingBarrier"`
}

type Barrier struct {
	Barrier int `json:"barrier"`
	Bps     int `json:"bps"`
}

type Target struct {
	Name         string          `json:"name"`
	EnemyPlayer  bool            `json:"enemyPlayer"`
	IsFakeTarget bool            `json:"isFake"`
	StatsAll     []TargetStats   `json:"statsAll"`
	DpsAll       []TargetDps     `json:"dpsAll"`
	Defenses     []TargetDefense `json:"defenses"`
}

type TargetStats struct {
	Dmg    int `json:"totaldmg"`
	Downed int `json:"downed"`
	Killed int `json:"killed"`
}

type TargetDps struct {
	Dps int `json:"dps"`
}

type TargetDefense struct {
	DownCount int `json:"downCount"`
	DeadCount int `json:"deadCount"`
}

type Mechanic struct {
	Name          string         `json:"name"`
	MechanicsData []MechanicData `json:"mechanicsData"`
}

type MechanicData struct {
	Time  int64  `json:"time"`
	Actor string `json:"actor"`
}

// ParseLog reads and unmarshals the JSON file from the given path.
func ParseLog(jsonPath string) (*ParsedLog, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, err
	}

	var log ParsedLog
	err = json.Unmarshal(data, &log)
	if err != nil {
		return nil, err
	}

	return &log, nil
}
