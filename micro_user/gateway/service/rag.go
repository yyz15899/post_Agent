package service

import (
	"context"
	"fmt"
	"log"
	"micro_user/gateway/dao"
	"micro_user/gateway/model"
	"micro_user/gateway/rag"

	milvusIndexer "github.com/cloudwego/eino-ext/components/indexer/milvus2"
)

// 入库(milvus)
// 同步入库(单篇post)
func IndexSinglePost(ctx context.Context, post model.Post, indexer *milvusIndexer.Indexer) error {
	doc := rag.PostToDocument([]model.Post{post})
	id, err := indexer.Store(ctx, doc)
	if err != nil {
		log.Printf("文档id:%v 入库失败:%v", id, err)
		return err
	}
	return nil
}

// 执行cmd时批量入库
func IndexPost(ctx context.Context, indexer *milvusIndexer.Indexer, limit int) ([]string, error) {
	posts, err := dao.ListPost(ctx, limit)
	if err != nil {
		log.Printf("dao.ListPost faile:%v", err)
		return nil, err
	}
	if len(posts) == 0 {
		return []string{}, nil
	}
	post_doc := rag.PostToDocument(posts)
	doc_id, err := indexer.Store(ctx, post_doc)
	if err != nil {
		log.Printf("indexer.Store failed：%v", err)
		return nil, err
	}
	return doc_id, nil

}

// 手动补写(mysql入库成功,milvus同步失败的情况)
func ReindexPost(ctx context.Context, indexer *milvusIndexer.Indexer, postid int64) error {
	post, err := GetPostByID(ctx, postid)
	if err != nil {
		return fmt.Errorf("查询帖子 %d 失败: %w", postid, err)
	}
	err = IndexSinglePost(ctx, *post, indexer)
	if err != nil {
		return fmt.Errorf("向量化或写入 Milvus 失败:err%w", err)
	}
	return nil

}
