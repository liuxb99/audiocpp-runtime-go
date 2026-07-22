單次 write 控制在 20-30KB UTF-8 以內。超過使用 ASCII 分片 + manifest + SHA256 驗證，每片 Base64 編碼後 < 20KB。
