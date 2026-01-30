namespace go base

struct Status {
    1: i64 code
    2: string msg
}

// User represents a full user profile
struct User {
    1: i64 user_id
    2: string user_name
    3: string email
    4: string password
    5: i64 sex
    6: string avatar_url
    7: optional string phone
    8: optional string background_url
    9: string bio
    10: optional string birthday
    11: optional string location
    12: optional i64 school_id
    13: i64 following_count
    14: i64 follower_count
    15: i64 like_count
    16: i64 video_count
    17: i64 status  // 1:normal 2:muted 3:banned
    18: string created_at
    19: string updated_at
    20: string deleted_at
}

// UserLite represents a minimal user info for display
struct UserLite {
    1: i64 uid
    2: string user_name
    3: string avatar_url
    4: optional string bio
    5: optional bool is_verified  // campus verified
}

// Video represents a video with all metadata
struct Video {
    1: i64 video_id
    2: i64 user_id
    3: string video_url
    4: string cover_url
    5: string title
    6: string description
    7: i64 visit_count
    8: i64 likes_count
    9: i64 comment_count
    10: string created_at
    11: string updated_at
    12: string deleted_at
    13: i64 open  // 0:private 1:public 2:friends only
    14: i64 audit_status  // 0:unreviewed 1:approved 2:rejected
    15: i64 share_count
    16: string label_names
    17: i64 favorites_count
    18: i64 history_count
    19: string category
    20: i64 duration  // video duration in seconds
    21: i64 width
    22: i64 height
    23: i64 file_size
    24: optional i64 school_id
    25: optional string location
    26: bool allow_comment
    27: bool allow_duet
    28: bool allow_download
}

// Comment represents a comment on a video
struct Comment {
    1: i64 comment_id
    2: i64 user_id
    3: i64 video_id
    4: i64 parent_id
    5: i64 like_count
    6: i64 child_count
    7: string content
    8: string created_at
    9: string updated_at
    10: string deleted_at
    11: i64 reply_to_comment_id  // target comment id for reply
    // User info fields for display
    12: optional string user_name      // commenter's username/nickname
    13: optional string avatar_url     // commenter's avatar URL
    14: optional i64 reply_to_user_id  // replied user's ID
    15: optional string reply_to_user_name  // replied user's username/nickname
}

// Favorite represents a favorites folder
struct Favorite {
    1: i64 favorite_id
    2: i64 user_id
    3: string name
    4: string description
    5: string cover_url
    6: i64 video_count
    7: bool is_public
    8: string created_at
    9: string updated_at
    10: string deleted_at
}

struct Recomendation {
    1: i64 video_id
    2: string title
    3: string description
    4: string label_names
    5: string category
}

// ========================================
// Campus Feature Structs
// ========================================

// School represents a school/university
struct School {
    1: i64 school_id
    2: string school_name
    3: string school_code
    4: string province
    5: string city
    6: optional string address
    7: i64 school_type  // 1:university 2:college 3:high school 4:other
    8: optional string logo_url
    9: optional string cover_url
    10: i64 student_count
    11: i64 video_count
    12: bool is_active
}

// UserVerification represents campus verification status
struct UserVerification {
    1: i64 id
    2: i64 user_id
    3: i64 school_id
    4: string student_id
    5: string real_name
    6: optional string department
    7: optional string major
    8: optional i64 enrollment_year
    9: optional i64 graduation_year
    10: i64 verification_status  // 0:unverified 1:pending 2:verified 3:failed 4:expired
    11: optional string rejection_reason
    12: optional string verified_at
    13: optional string expire_at
}

// Topic represents a topic or challenge
struct Topic {
    1: i64 topic_id
    2: string title
    3: optional string description
    4: optional string cover_url
    5: i64 creator_id
    6: i64 topic_type  // 1:normal 2:challenge 3:campus activity 4:official
    7: optional i64 school_id
    8: i64 participate_count
    9: i64 view_count
    10: i64 status  // 1:normal 2:hot 3:banned 4:ended
    11: optional string start_time
    12: optional string end_time
    13: optional string prize_info
    14: optional string rules
}

// ========================================
// Social Feature Structs
// ========================================

// DirectMessage represents a private message
struct DirectMessage {
    1: i64 message_id
    2: i64 conversation_id
    3: i64 sender_id
    4: i64 receiver_id
    5: string content
    6: i64 message_type  // 1:text 2:image 3:video 4:share 5:emoji
    7: optional i64 related_video_id
    8: bool is_read
    9: optional string read_at
    10: string created_at
}

// Conversation represents a chat conversation
struct Conversation {
    1: i64 conversation_id
    2: i64 user_id_1
    3: i64 user_id_2
    4: optional i64 last_message_id
    5: optional string last_message_content
    6: optional string last_message_time
    7: i64 user_1_unread_count
    8: i64 user_2_unread_count
}

// Notification represents a user notification
struct Notification {
    1: i64 notification_id
    2: i64 user_id
    3: optional i64 sender_id
    4: i64 notification_type  // 1:like 2:comment 3:follow 4:mention 5:system 6:activity
    5: optional i64 target_type  // 1:video 2:comment 3:user
    6: optional i64 target_id
    7: optional string title
    8: optional string content
    9: bool is_read
    10: optional string read_at
    11: string created_at
}

// Report represents a user report
struct Report {
    1: i64 report_id
    2: i64 reporter_id
    3: i64 target_type  // 1:video 2:comment 3:user 4:message
    4: i64 target_id
    5: i64 reason_type  // 1:porn 2:violence 3:illegal 4:spam 5:fraud 6:other
    6: optional string reason_detail
    7: i64 status  // 0:pending 1:processing 2:resolved 3:rejected
    8: string created_at
}

// SearchHistory represents a search record
struct SearchHistory {
    1: i64 id
    2: i64 user_id
    3: string keyword
    4: i64 search_type  // 1:all 2:user 3:video 4:topic 5:school
    5: i64 result_count
    6: string created_at
}

// HotSearch represents hot search keywords
struct HotSearch {
    1: i64 id
    2: string keyword
    3: i64 search_count
    4: double heat_score
    5: optional string category
    6: bool is_promoted
    7: optional i64 rank_position
}

// Blacklist represents blocked users
struct Blacklist {
    1: i64 id
    2: i64 user_id
    3: i64 blocked_user_id
    4: optional string reason
    5: string created_at
}
