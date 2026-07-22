# BLE 协议 v1

## 广播

手机使用个人用途的厂商标识 `0xFFFF`，在 BLE 厂商数据中广播 13 字节。Windows
的 tinygo WinRT 后端不提供 Service Data 段，因此协议使用厂商数据：

```text
version(1) || random_salt(4) || HMAC-SHA256(presence_key,
"ProximityUnlock/ad/v1" || random_salt)[0:8]
```

配对广播使用 `version | 0x80`、PC ID 的前四字节以及八字节配对 HMAC。协议从不
信任蓝牙地址或设备名称。

## GATT

- 服务：`9b7c6a10-5d57-4c2e-8e2a-4ed2f5f7a001`
- 写入挑战：`...a002`
- 响应通知：`...a003`
- 配对写入/通知：`...a004`

消息体使用确定性大端二进制编码。ECDSA 签名采用 64 字节 IEEE P1363
`r || s` 格式。GATT 数据以三字节
`message_type || zero_based_index || fragment_count` 头部分片，每个分片最多
180 字节。

每次解锁挑战会绑定协议版本、模式、PC ID、手机 ID、目标 SID 哈希、Windows
会话、签发和过期时间，以及一个 256 位随机数。手机先验证电脑签名，再对挑战
摘要、随机数、单调计数器、双方身份、模式和签名时间进行签名。电脑会拒绝重复
随机数以及未递增的计数器。

## 配对

配对二维码包含协议版本、PC ID、PC 公钥、256 位一次性密钥和两分钟有效期。
手机返回安全模式与便捷模式各自的公钥。双方通过 HKDF 派生 presence key，随后
清除一次性配对密钥。广播只携带滚动盐和截断 HMAC，不暴露静态手机标识。

## 解锁授权

每次挑战均使用新的 256 位 nonce。手机响应同时绑定目标会话、模式、有效期和
计数器，五秒后失效。Windows 服务验证成功后只创建一次性授权；凭据提供程序
消费后立即删除，不能重复使用。
