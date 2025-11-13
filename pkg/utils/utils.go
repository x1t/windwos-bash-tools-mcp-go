package utils

// IntPtr 返回int的指针
func IntPtr(i int) *int {
	return &i
}

// BoolPtr 返回bool的指针
func BoolPtr(b bool) *bool {
	return &b
}

// StringPtr 返回string的指针
func StringPtr(s string) *string {
	return &s
}