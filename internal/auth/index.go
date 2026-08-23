package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"time"

	"github.com/redis/go-redis/v9"
)

type redisSessionIndex struct {
	client        *redis.Client
	prefix        string
	sessionPrefix string
	retention     time.Duration
}

// newRedisSessionIndex 创建用户和设备到会话摘要的反向索引
func newRedisSessionIndex(
	client *redis.Client,
	prefix string,
	sessionPrefix string,
	retention time.Duration,
) *redisSessionIndex {
	return &redisSessionIndex{
		client:        client,
		prefix:        prefix,
		sessionPrefix: sessionPrefix,
		retention:     retention,
	}
}

func (i *redisSessionIndex) add(
	ctx context.Context,
	userID string,
	device string,
	tokenHash string,
) error {
	userSessionsKey := i.userSessionsKey(userID)
	userDevicesKey := i.userDevicesKey(userID)
	deviceSessionsKey := i.deviceSessionsKey(userID, device)
	encodedDevice := encodeKeyPart(device)

	// 原子记录用户和设备到会话的反向索引
	_, err := i.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.SAdd(ctx, userSessionsKey, tokenHash)
		pipe.SAdd(ctx, userDevicesKey, encodedDevice)
		pipe.SAdd(ctx, deviceSessionsKey, tokenHash)
		pipe.Expire(ctx, userSessionsKey, i.retention)
		pipe.Expire(ctx, userDevicesKey, i.retention)
		pipe.Expire(ctx, deviceSessionsKey, i.retention)
		return nil
	})
	return err
}

func (i *redisSessionIndex) remove(
	ctx context.Context,
	userID string,
	device string,
	tokenHash string,
) error {
	// 当前会话失效后同步移除用户和设备索引
	_, err := i.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.SRem(ctx, i.userSessionsKey(userID), tokenHash)
		pipe.SRem(ctx, i.deviceSessionsKey(userID, device), tokenHash)
		return nil
	})
	return err
}

func (i *redisSessionIndex) deleteDevice(ctx context.Context, userID string, device string) error {
	deviceSessionsKey := i.deviceSessionsKey(userID, device)
	tokenHashes, err := i.client.SMembers(ctx, deviceSessionsKey).Result()
	if err != nil {
		return err
	}

	// 原子删除指定设备的会话数据和反向索引
	_, err = i.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		deleteStoredSessions(ctx, pipe, i.sessionPrefix, tokenHashes)
		if len(tokenHashes) > 0 {
			pipe.SRem(ctx, i.userSessionsKey(userID), stringMembers(tokenHashes)...)
		}
		pipe.Del(ctx, deviceSessionsKey)
		pipe.SRem(ctx, i.userDevicesKey(userID), encodeKeyPart(device))
		return nil
	})
	return err
}

func (i *redisSessionIndex) deleteUser(ctx context.Context, userID string) error {
	tokenHashes, err := i.client.SMembers(ctx, i.userSessionsKey(userID)).Result()
	if err != nil {
		return err
	}
	devices, err := i.client.SMembers(ctx, i.userDevicesKey(userID)).Result()
	if err != nil {
		return err
	}

	indexKeys := []string{i.userSessionsKey(userID), i.userDevicesKey(userID)}
	for _, device := range devices {
		indexKeys = append(indexKeys, i.encodedDeviceSessionsKey(userID, device))
	}

	// 原子删除当前登录域内指定用户的全部会话
	_, err = i.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		deleteStoredSessions(ctx, pipe, i.sessionPrefix, tokenHashes)
		pipe.Del(ctx, indexKeys...)
		return nil
	})
	return err
}

func (i *redisSessionIndex) userSessionsKey(userID string) string {
	return i.prefix + "user:" + encodeKeyPart(userID) + ":sessions"
}

func (i *redisSessionIndex) userDevicesKey(userID string) string {
	return i.prefix + "user:" + encodeKeyPart(userID) + ":devices"
}

func (i *redisSessionIndex) deviceSessionsKey(userID string, device string) string {
	return i.encodedDeviceSessionsKey(userID, encodeKeyPart(device))
}

func (i *redisSessionIndex) encodedDeviceSessionsKey(userID string, encodedDevice string) string {
	return i.prefix + "user:" + encodeKeyPart(userID) + ":device:" + encodedDevice + ":sessions"
}

// deleteStoredSessions 批量删除 SCS 保存的摘要会话键
func deleteStoredSessions(
	ctx context.Context,
	pipe redis.Pipeliner,
	sessionPrefix string,
	tokenHashes []string,
) {
	if len(tokenHashes) == 0 {
		return
	}
	keys := make([]string, 0, len(tokenHashes))
	for _, tokenHash := range tokenHashes {
		keys = append(keys, sessionPrefix+tokenHash)
	}
	pipe.Del(ctx, keys...)
}

// tokenDigest 使用与 SCS 一致的算法计算令牌摘要
func tokenDigest(token string) string {
	hash := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// encodeKeyPart 将外部标识编码为安全的 Redis 键片段
func encodeKeyPart(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

// stringMembers 将字符串切片转换为 Redis 命令参数
func stringMembers(values []string) []any {
	members := make([]any, 0, len(values))
	for _, value := range values {
		members = append(members, value)
	}
	return members
}
