package rag

import (
	"micro_user/gateway/model"
	"strconv"

	"github.com/cloudwego/eino/schema"
)

// 将数据库中数据转换为文档形式
func PostToDocument(post []model.Post) []*schema.Document {
	docs := make([]*schema.Document, 0, len(post))

	for _, p := range post {
		doc := &schema.Document{
			ID:      "post_" + strconv.FormatInt(p.Id, 10),
			Content: p.Title + "\n" + p.Content,
			MetaData: map[string]any{
				"post_id": p.Id,
				"title":   p.Title,
			},
		}
		docs = append(docs, doc)
	}
	return docs
}
