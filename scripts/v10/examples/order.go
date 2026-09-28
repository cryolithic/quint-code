package cancel

func CanCancel(status string) bool {
	return status == "pending"
}
