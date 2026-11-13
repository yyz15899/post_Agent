package service

func main() {

}

type User struct {
	ID  int
	AGE int
}

// 根据id去重(通过将id写到map中来实现去重拿到id从而获取其他信息)
func UniqueUsersByID(users []User) []User {
	seen := make(map[int]struct{})
	result := make([]User, 0, len(seen))
	for _, u := range users {
		seen[u.ID] = struct{}{}
	}
	for user_id := range seen { // 单变量遍历时,拿到key 即u.ID
		result = append(result, User{ID: user_id})
	}
	return result
}
