class User {
  const User({
    required this.id,
    required this.username,
    this.lastSeenAt,
    this.online = false,
  });

  final String id;
  final String username;
  final DateTime? lastSeenAt;
  final bool online;

  factory User.fromJson(Map<String, dynamic> j) => User(
    id: j['id'] as String,
    username: j['username'] as String,
    lastSeenAt: _parseTime(j['last_seen_at']),
    online: j['online'] as bool? ?? false,
  );

  Map<String, dynamic> toJson() => {
    'id': id,
    'username': username,
    'last_seen_at': lastSeenAt?.toUtc().toIso8601String(),
  };

  User copyWith({bool? online, DateTime? lastSeenAt}) => User(
    id: id,
    username: username,
    online: online ?? this.online,
    lastSeenAt: lastSeenAt ?? this.lastSeenAt,
  );
}

enum MessageStatus { pending, sent, failed }

class Message {
  const Message({
    required this.id,
    required this.clientId,
    required this.dialogId,
    required this.senderId,
    required this.body,
    required this.createdAt,
    this.status = MessageStatus.sent,
  });

  /// Server-assigned; null while the message is still pending.
  final int? id;
  final String clientId;
  final String dialogId;
  final String senderId;
  final String body;

  /// Server time for sent messages, local time for pending ones.
  final DateTime createdAt;
  final MessageStatus status;

  factory Message.fromJson(Map<String, dynamic> j) => Message(
    id: j['id'] as int,
    clientId: j['client_id'] as String,
    dialogId: j['dialog_id'] as String,
    senderId: j['sender_id'] as String,
    body: j['body'] as String,
    createdAt: _parseTime(j['created_at'])!,
  );

  Message copyWith({MessageStatus? status}) => Message(
    id: id,
    clientId: clientId,
    dialogId: dialogId,
    senderId: senderId,
    body: body,
    createdAt: createdAt,
    status: status ?? this.status,
  );
}

class Conversation {
  const Conversation({
    required this.id,
    required this.peer,
    this.lastMessage,
    this.unreadCount = 0,
    this.myLastReadId = 0,
    this.peerLastReadId = 0,
  });

  final String id;
  final User peer;
  final Message? lastMessage;
  final int unreadCount;
  final int myLastReadId;
  final int peerLastReadId;

  factory Conversation.fromJson(Map<String, dynamic> j) => Conversation(
    id: j['id'] as String,
    peer: User.fromJson(j['peer'] as Map<String, dynamic>),
    lastMessage: j['last_message'] == null
        ? null
        : Message.fromJson(j['last_message'] as Map<String, dynamic>),
    unreadCount: j['unread_count'] as int,
    myLastReadId: j['my_last_read_id'] as int,
    peerLastReadId: j['peer_last_read_id'] as int,
  );

  Conversation copyWith({
    User? peer,
    Message? lastMessage,
    int? unreadCount,
    int? myLastReadId,
    int? peerLastReadId,
  }) => Conversation(
    id: id,
    peer: peer ?? this.peer,
    lastMessage: lastMessage ?? this.lastMessage,
    unreadCount: unreadCount ?? this.unreadCount,
    myLastReadId: myLastReadId ?? this.myLastReadId,
    peerLastReadId: peerLastReadId ?? this.peerLastReadId,
  );
}

/// The server sends UTC; the UI always works in local time.
DateTime? _parseTime(Object? v) =>
    v == null ? null : DateTime.parse(v as String).toLocal();
