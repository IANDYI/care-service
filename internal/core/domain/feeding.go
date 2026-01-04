package domain

// ValidBreastfeedingPositions returns all valid breastfeeding positions
func ValidBreastfeedingPositions() []BreastfeedingPosition {
	return []BreastfeedingPosition{
		PositionCrossCradle,
		PositionCradle,
		PositionFootball,
		PositionSideLying,
		PositionLaidBack,
	}
}

// IsValidBreastfeedingPosition checks if a position is valid
func IsValidBreastfeedingPosition(position BreastfeedingPosition) bool {
	validPositions := ValidBreastfeedingPositions()
	for _, p := range validPositions {
		if p == position {
			return true
		}
	}
	return false
}

// ValidBreastfeedingSides returns all valid breastfeeding sides
func ValidBreastfeedingSides() []BreastfeedingSide {
	return []BreastfeedingSide{
		SideLeft,
		SideRight,
		SideBoth,
	}
}

// IsValidBreastfeedingSide checks if a side is valid
func IsValidBreastfeedingSide(side BreastfeedingSide) bool {
	validSides := ValidBreastfeedingSides()
	for _, s := range validSides {
		if s == side {
			return true
		}
	}
	return false
}

// ValidDiaperStatuses returns all valid diaper statuses
func ValidDiaperStatuses() []DiaperStatus {
	return []DiaperStatus{
		DiaperStatusDry,
		DiaperStatusWet,
		DiaperStatusDirty,
		DiaperStatusBoth,
	}
}

// IsValidDiaperStatus checks if a diaper status is valid
func IsValidDiaperStatus(status DiaperStatus) bool {
	validStatuses := ValidDiaperStatuses()
	for _, s := range validStatuses {
		if s == status {
			return true
		}
	}
	return false
}


