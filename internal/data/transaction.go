package data

import (
	"context"

	"gorm.io/gorm"
)

type transactionContextKey struct{}

// WithinTransaction 在事务中执行函数并通过上下文传递事务会话
func (d *Data) WithinTransaction(ctx context.Context, function func(context.Context) error) error {
	return d.DB(ctx).Transaction(func(transaction *gorm.DB) error {
		// 将事务会话写入新上下文供多个数据操作共享
		return function(withTransaction(ctx, transaction))
	})
}

// withTransaction 创建携带 GORM 事务会话的上下文
func withTransaction(ctx context.Context, transaction *gorm.DB) context.Context {
	return context.WithValue(ctx, transactionContextKey{}, transaction)
}

// transactionFromContext 读取当前业务事务会话
func transactionFromContext(ctx context.Context) (*gorm.DB, bool) {
	transaction, ok := ctx.Value(transactionContextKey{}).(*gorm.DB)
	return transaction, ok
}
