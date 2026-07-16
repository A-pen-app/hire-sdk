package models

import "strconv"

// Sentinel values for the apen absolute-tenure encoding used by
// BusinessCardContent.ExperienceYears (and user.experience_years in apen's
// main DB). Values in between are literal year counts.
const (
	ExperienceYearsLessThanOne = 0  // "1年以下"; also covers no-experience and students
	ExperienceYearsTwentyPlus  = 21 // "20年以上"; never a literal year count
)

// FormatExperienceYears decodes the sentinel encoding into the display
// string. All render paths (chat card, snapshots) must go through this
// instead of printing the raw value.
func FormatExperienceYears(v int) string {
	switch {
	case v <= ExperienceYearsLessThanOne:
		return "1年以下"
	case v >= ExperienceYearsTwentyPlus:
		return "20年以上"
	default:
		return strconv.Itoa(v) + "年"
	}
}

// ExperienceRange is the nurse / phar range-based tenure stored in
// BusinessCardContent.ExperienceRange (and user.experience_range in their
// main DBs). Stored as strings so snapshots stay self-describing.
type ExperienceRange string

const (
	ExperienceRangeLessThanOne  ExperienceRange = "LESS_THAN_ONE"  // 1年以下
	ExperienceRangeOneToThree   ExperienceRange = "ONE_TO_THREE"   // 1-3年
	ExperienceRangeThreeToFive  ExperienceRange = "THREE_TO_FIVE"  // 3-5年
	ExperienceRangeFiveToTen    ExperienceRange = "FIVE_TO_TEN"    // 5-10年
	ExperienceRangeTenToFifteen ExperienceRange = "TEN_TO_FIFTEEN" // 10-15年
	ExperienceRangeFifteenPlus  ExperienceRange = "FIFTEEN_PLUS"   // 15年以上
)

func (e ExperienceRange) Chinese() string {
	switch e {
	case ExperienceRangeLessThanOne:
		return "1年以下"
	case ExperienceRangeOneToThree:
		return "1-3年"
	case ExperienceRangeThreeToFive:
		return "3-5年"
	case ExperienceRangeFiveToTen:
		return "5-10年"
	case ExperienceRangeTenToFifteen:
		return "10-15年"
	case ExperienceRangeFifteenPlus:
		return "15年以上"
	}
	return ""
}

func ValidateExperienceRange(e ExperienceRange) bool {
	return e.Chinese() != ""
}
