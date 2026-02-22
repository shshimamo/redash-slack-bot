-- テーブル作成
-- configs/queries.yaml のクエリを確認するためのサンプルデータ

-- 決済テーブル
CREATE TABLE IF NOT EXISTS payments (
    id SERIAL PRIMARY KEY,
    request_id VARCHAR(50) NOT NULL,
    user_id VARCHAR(50),
    amount DECIMAL(10, 2) NOT NULL,
    status VARCHAR(20) NOT NULL,
    payment_method VARCHAR(30),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP
);

-- 決済ログテーブル
CREATE TABLE IF NOT EXISTS payment_logs (
    id SERIAL PRIMARY KEY,
    request_id VARCHAR(50) NOT NULL,
    log_level VARCHAR(10) NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 決済エラーテーブル
CREATE TABLE IF NOT EXISTS payment_errors (
    id SERIAL PRIMARY KEY,
    request_id VARCHAR(50) NOT NULL,
    error_code VARCHAR(20) NOT NULL,
    error_message TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- ユーザープロフィールテーブル（別サービスを想定）
CREATE TABLE IF NOT EXISTS user_profiles (
    user_id VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    email VARCHAR(100) NOT NULL,
    plan_type VARCHAR(20) NOT NULL,
    registered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- 決済詳細テーブル（ゲートウェイの処理結果など）
CREATE TABLE IF NOT EXISTS payment_details (
    id SERIAL PRIMARY KEY,
    payment_id INT NOT NULL,
    gateway VARCHAR(50) NOT NULL,
    response_code VARCHAR(10),
    masked_card VARCHAR(20),
    processed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- システムステータステーブル
CREATE TABLE IF NOT EXISTS system_status (
    id SERIAL PRIMARY KEY,
    status VARCHAR(20) NOT NULL,
    cpu_usage DECIMAL(5, 2),
    memory_usage DECIMAL(5, 2),
    active_connections INT,
    checked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- インデックス作成
CREATE INDEX IF NOT EXISTS idx_payments_request_id ON payments(request_id);
CREATE INDEX IF NOT EXISTS idx_payments_created_at ON payments(created_at);
CREATE INDEX IF NOT EXISTS idx_payment_logs_request_id ON payment_logs(request_id);
CREATE INDEX IF NOT EXISTS idx_payment_errors_request_id ON payment_errors(request_id);
CREATE INDEX IF NOT EXISTS idx_payment_details_payment_id ON payment_details(payment_id);

-- テーブルコメント
COMMENT ON TABLE payments IS '決済情報を管理するテーブル';
COMMENT ON TABLE payment_logs IS '決済処理のログを記録するテーブル';
COMMENT ON TABLE payment_errors IS '決済エラー情報を記録するテーブル';
COMMENT ON TABLE system_status IS 'システムの稼働状況を記録するテーブル';
COMMENT ON TABLE user_profiles IS 'ユーザープロフィール（別サービス想定）';
COMMENT ON TABLE payment_details IS '決済ゲートウェイの処理詳細';

-- payments カラムコメント
COMMENT ON COLUMN payments.id IS '決済ID（主キー）';
COMMENT ON COLUMN payments.request_id IS 'リクエストID（外部システムとの連携用）';
COMMENT ON COLUMN payments.user_id IS 'ユーザーID';
COMMENT ON COLUMN payments.amount IS '決済金額';
COMMENT ON COLUMN payments.status IS 'ステータス（pending/completed/failed/refunded）';
COMMENT ON COLUMN payments.payment_method IS '決済方法（credit_card/bank_transfer/convenience_store）';
COMMENT ON COLUMN payments.created_at IS '作成日時';
COMMENT ON COLUMN payments.updated_at IS '更新日時';

-- payment_logs カラムコメント
COMMENT ON COLUMN payment_logs.id IS 'ログID（主キー）';
COMMENT ON COLUMN payment_logs.request_id IS 'リクエストID';
COMMENT ON COLUMN payment_logs.log_level IS 'ログレベル（INFO/WARN/ERROR）';
COMMENT ON COLUMN payment_logs.message IS 'ログメッセージ';
COMMENT ON COLUMN payment_logs.created_at IS '記録日時';

-- payment_errors カラムコメント
COMMENT ON COLUMN payment_errors.id IS 'エラーID（主キー）';
COMMENT ON COLUMN payment_errors.request_id IS 'リクエストID';
COMMENT ON COLUMN payment_errors.error_code IS 'エラーコード';
COMMENT ON COLUMN payment_errors.error_message IS 'エラーメッセージ';
COMMENT ON COLUMN payment_errors.created_at IS '発生日時';

-- system_status カラムコメント
COMMENT ON COLUMN system_status.id IS 'ステータスID（主キー）';
COMMENT ON COLUMN system_status.status IS 'システム状態（healthy/warning/critical）';
COMMENT ON COLUMN system_status.cpu_usage IS 'CPU使用率（%）';
COMMENT ON COLUMN system_status.memory_usage IS 'メモリ使用率（%）';
COMMENT ON COLUMN system_status.active_connections IS 'アクティブ接続数';
COMMENT ON COLUMN system_status.checked_at IS 'チェック日時';

-- サンプルデータ投入

-- ユーザープロフィールデータ
INSERT INTO user_profiles (user_id, name, email, plan_type, registered_at) VALUES
('user_001', '山田 太郎', 'taro@example.com', 'premium', '2023-06-01 09:00:00'),
('user_002', '鈴木 花子', 'hanako@example.com', 'standard', '2023-08-15 14:30:00'),
('user_003', '田中 一郎', 'ichiro@example.com', 'standard', '2023-10-20 11:00:00'),
('user_004', '佐藤 美咲', 'misaki@example.com', 'premium', '2023-12-01 10:00:00'),
('user_005', '伊藤 健', 'ken@example.com', 'standard', '2024-01-05 08:00:00');

-- 決済データ（user_id 付き）
INSERT INTO payments (request_id, user_id, amount, status, payment_method, created_at) VALUES
('req_abc123', 'user_001', 1500.00, 'completed', 'credit_card', '2024-01-15 10:30:00'),
('req_def456', 'user_002', 3200.00, 'completed', 'bank_transfer', '2024-01-15 11:45:00'),
('req_ghi789', 'user_003', 800.00, 'completed', 'credit_card', '2024-01-16 09:00:00'),
('req_jkl012', 'user_001', 5000.00, 'pending', 'bank_transfer', '2024-01-16 14:20:00'),
('req_mno345', 'user_004', 1200.00, 'failed', 'credit_card', '2024-01-17 16:30:00'),
('req_pqr678', 'user_002', 2500.00, 'completed', 'credit_card', '2024-01-18 08:15:00'),
('req_stu901', 'user_003', 4800.00, 'refunded', 'credit_card', '2024-01-18 12:00:00'),
('req_vwx234', 'user_005', 990.00, 'completed', 'convenience_store', '2024-01-19 20:30:00'),
('req_yza567', 'user_004', 15000.00, 'pending', 'bank_transfer', '2024-01-20 10:00:00'),
('req_bcd890', 'user_005', 3300.00, 'failed', 'credit_card', '2024-01-20 15:45:00');

-- 決済ログデータ
INSERT INTO payment_logs (request_id, log_level, message, created_at) VALUES
-- req_abc123 の正常フロー
('req_abc123', 'INFO', '決済リクエスト受信', '2024-01-15 10:30:00'),
('req_abc123', 'INFO', 'クレジットカード認証開始', '2024-01-15 10:30:01'),
('req_abc123', 'INFO', 'クレジットカード認証成功', '2024-01-15 10:30:03'),
('req_abc123', 'INFO', '決済処理完了', '2024-01-15 10:30:05'),
-- req_mno345 の失敗フロー
('req_mno345', 'INFO', '決済リクエスト受信', '2024-01-17 16:30:00'),
('req_mno345', 'INFO', 'クレジットカード認証開始', '2024-01-17 16:30:01'),
('req_mno345', 'ERROR', 'クレジットカード認証失敗: カード利用限度額超過', '2024-01-17 16:30:03'),
('req_mno345', 'INFO', '決済処理中止', '2024-01-17 16:30:04'),
-- req_stu901 の返金フロー
('req_stu901', 'INFO', '決済リクエスト受信', '2024-01-18 12:00:00'),
('req_stu901', 'INFO', '決済処理完了', '2024-01-18 12:00:05'),
('req_stu901', 'INFO', '返金リクエスト受信', '2024-01-18 14:30:00'),
('req_stu901', 'INFO', '返金処理完了', '2024-01-18 14:30:10');

-- 決済エラーデータ
INSERT INTO payment_errors (request_id, error_code, error_message, created_at) VALUES
('req_mno345', 'CARD_LIMIT_EXCEEDED', 'カードの利用限度額を超過しています', '2024-01-17 16:30:03'),
('req_bcd890', 'CARD_EXPIRED', 'カードの有効期限が切れています', '2024-01-20 15:45:02'),
('req_bcd890', 'PAYMENT_DECLINED', '決済が拒否されました', '2024-01-20 15:45:03');

-- 決済詳細データ（payments.id に対応）
-- payments の id は INSERT 順に 1〜10 が割り当てられる想定
INSERT INTO payment_details (payment_id, gateway, response_code, masked_card, processed_at) VALUES
(1, 'stripe',  '00', '**** **** **** 1234', '2024-01-15 10:30:03'),
(2, 'payjp',   '00', NULL,                  '2024-01-15 11:45:10'),
(3, 'stripe',  '00', '**** **** **** 5678', '2024-01-16 09:00:02'),
(4, 'payjp',   NULL, NULL,                  '2024-01-16 14:20:00'),
(5, 'stripe',  '51', '**** **** **** 9012', '2024-01-17 16:30:03'),
(6, 'stripe',  '00', '**** **** **** 3456', '2024-01-18 08:15:02'),
(7, 'stripe',  '00', '**** **** **** 7890', '2024-01-18 12:00:03'),
(8, 'payjp',   '00', NULL,                  '2024-01-19 20:30:05'),
(9, 'payjp',   NULL, NULL,                  '2024-01-20 10:00:00'),
(10, 'stripe', '54', '**** **** **** 2345', '2024-01-20 15:45:02');

-- システムステータスデータ
INSERT INTO system_status (status, cpu_usage, memory_usage, active_connections, checked_at) VALUES
('healthy', 25.5, 48.2, 150, '2024-01-20 10:00:00'),
('healthy', 30.2, 52.1, 180, '2024-01-20 11:00:00'),
('warning', 75.8, 68.5, 350, '2024-01-20 12:00:00'),
('healthy', 35.0, 55.0, 200, '2024-01-20 13:00:00'),
('healthy', 28.3, 50.8, 175, '2024-01-20 14:00:00');
