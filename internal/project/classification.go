package project

// DetectType classifies a newly added content directory using the same markers
// as automatic Book discovery. Existing Projects keep their stored type.
func DetectType(path string) (Type, error) {
	canonical, err := canonicalDirectory(path, true)
	if err != nil {
		return "", err
	}
	if isBookWorkspace(canonical) {
		return TypeBook, nil
	}
	return TypeGeneral, nil
}
