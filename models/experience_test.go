package models

import "testing"

// Locks the legacy 12-step ordinals in place: existing rows in
// social_business_card.year_of_experience and hire DB hospital_experience
// store these values, so any shift silently corrupts every stored tenure.
func TestYearOfExperienceOrdinalsAreStable(t *testing.T) {
	want := []struct {
		value   YearOfExperienceType
		ordinal int
		chinese string
	}{
		{YearOfExperienceNone, 0, "無經驗"},
		{YearOfExperienceLessThanOne, 1, "1年以下"},
		{YearOfExperienceOneToTwo, 2, "1年 ~ 2年"},
		{YearOfExperienceTwoToThree, 3, "2年 ~ 3年"},
		{YearOfExperienceThreeToFour, 4, "3年 ~ 4年"},
		{YearOfExperienceFourToFive, 5, "4年 ~ 5年"},
		{YearOfExperienceFiveToSix, 6, "5年 ~ 6年"},
		{YearOfExperienceSixToSeven, 7, "6年 ~ 7年"},
		{YearOfExperienceSevenToEight, 8, "7年 ~ 8年"},
		{YearOfExperienceEightToNine, 9, "8年 ~ 9年"},
		{YearOfExperienceNineToTen, 10, "9年 ~ 10年"},
		{YearOfExperienceMoreThanTen, 11, "10年以上"},
	}
	for _, w := range want {
		if int(w.value) != w.ordinal {
			t.Errorf("ordinal shifted: %s = %d, want %d", w.chinese, w.value, w.ordinal)
		}
		if got := w.value.Chinese(); got != w.chinese {
			t.Errorf("Chinese(%d) = %q, want %q", w.value, got, w.chinese)
		}
	}
}

func TestValidateYearOfExperience(t *testing.T) {
	for y := YearOfExperienceNone; y <= YearOfExperienceMoreThanTen; y++ {
		if !ValidateYearOfExperience(y) {
			t.Errorf("ValidateYearOfExperience(%d) = false, want true", y)
		}
	}
	for _, y := range []YearOfExperienceType{-1, 12, 99} {
		if ValidateYearOfExperience(y) {
			t.Errorf("ValidateYearOfExperience(%d) = true, want false", y)
		}
	}
}

func TestFormatExperienceYears(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{ExperienceYearsLessThanOne, "1年以下"},
		{1, "1年"},
		{7, "7年"},
		{20, "20年"},
		{ExperienceYearsTwentyPlus, "20年以上"},
		{-1, "1年以下"},  // defensive: below range clamps to the low sentinel
		{30, "20年以上"}, // defensive: above range clamps to the high sentinel
	}
	for _, c := range cases {
		if got := FormatExperienceYears(c.in); got != c.want {
			t.Errorf("FormatExperienceYears(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExperienceRange(t *testing.T) {
	want := map[ExperienceRange]string{
		ExperienceRangeLessThanOne:  "1年以下",
		ExperienceRangeOneToThree:   "1-3年",
		ExperienceRangeThreeToFive:  "3-5年",
		ExperienceRangeFiveToTen:    "5-10年",
		ExperienceRangeTenToFifteen: "10-15年",
		ExperienceRangeFifteenPlus:  "15年以上",
	}
	for r, chinese := range want {
		if !ValidateExperienceRange(r) {
			t.Errorf("ValidateExperienceRange(%q) = false, want true", r)
		}
		if got := r.Chinese(); got != chinese {
			t.Errorf("Chinese(%q) = %q, want %q", r, got, chinese)
		}
	}
	for _, r := range []ExperienceRange{"", "less_than_one", "TWENTY_PLUS"} {
		if ValidateExperienceRange(r) {
			t.Errorf("ValidateExperienceRange(%q) = true, want false", r)
		}
	}
}
