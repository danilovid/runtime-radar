/**
 * Who wrote a message. The server accepts these two roles only: tool traffic
 * belongs to the assistant loop and is never replayed by the client.
 */
export enum AssistantRole {
    ASSISTANT = 'assistant',
    USER = 'user'
}

/**
 * How a finished answer ended.
 */
export enum AssistantStopReason {
    CONFIRMATION_REQUIRED = 'confirmation_required',
    END_TURN = 'end_turn',
    ERROR = 'error',
    MAX_ITERATIONS = 'max_iterations'
}

/**
 * What the assistant was opened to do. The server gives itself different
 * instructions per mode; it never gains new powers from one.
 */
export enum AssistantMode {
    CHAT = 'chat',
    DIGEST = 'digest',
    EXPLAIN = 'explain',
    SUPPORT = 'support'
}

/**
 * Where a proposed action stands. Nothing is run until the user approves it,
 * and an approval is spent the moment it is used.
 */
export enum AssistantActionState {
    APPROVED = 'approved',
    DECLINED = 'declined',
    FAILED = 'failed',
    PENDING = 'pending'
}

/**
 * Phases of a tool the assistant runs while answering.
 */
export enum AssistantToolPhase {
    FINISHED = 'finished',
    STARTED = 'started'
}

/**
 * Which screen the widget shows.
 */
export enum AssistantView {
    CHAT = 'chat',
    HOME = 'home',
    QUIZ = 'quiz',
    TOUR = 'tour'
}

/**
 * One tool the assistant ran, in the order it was started. It is what the
 * "assistant is calling search_docs…" indicator is built from.
 */
export interface AssistantToolActivity {
    name: string;
    isRunning: boolean;
    error?: string;
}

/**
 * A change to the product the assistant wants to make and may not make alone.
 * It is shown with the arguments the model wrote, verbatim: the user approves
 * the call that will run, not a description of it.
 */
export interface AssistantAction {
    id: string;
    tool: string;
    title: string;
    /** Pretty-printed JSON the tool would be called with. */
    arguments: string;
    /** Set when the call removes something rather than adds it. */
    isDestructive: boolean;
    state: AssistantActionState;
}

/**
 * A credential an approved action produced. It reaches the browser through the
 * answer stream and never through the model, and it is shown once: the product
 * stores only a hash of it.
 */
export interface AssistantSecret {
    label: string;
    value: string;
    note: string;
}

/**
 * The support request the assistant wrote, pulled out of the answer so that the
 * interface can offer to send it. See the support mode in notifier/README.md.
 */
export interface AssistantSupportRequest {
    subject: string;
    body: string;
}

/**
 * A file the user attached to a question. Files are read in the browser and
 * travel as text inside the message, so nothing is uploaded or stored.
 */
export interface AssistantAttachment {
    name: string;
    size: number;
    content: string;
    isTruncated: boolean;
}

/**
 * One turn of the conversation.
 */
export interface AssistantMessage {
    id: string;
    role: AssistantRole;
    content: string;
    tools: AssistantToolActivity[];
    attachments: AssistantAttachment[];
    /** The change the assistant is asking to make, when it asked for one. */
    action?: AssistantAction;
    /** Credentials the approved action produced, shown once. */
    secrets: AssistantSecret[];
    /** Set while the answer is still being streamed. */
    isPending: boolean;
    /** Set when the answer could not be produced or was cut short. */
    error?: string;
    stopReason?: AssistantStopReason;
}

/**
 * A conversation. Conversations live in the store for as long as the page
 * does; the chats page lists them from there.
 */
export interface AssistantConversation {
    id: string;
    title: string;
    createdAt: string;
    updatedAt: string;
    messages: AssistantMessage[];
    /** The runtime event this conversation was opened from, if any. */
    eventId: string;
    /** What this conversation was opened to do. */
    mode: AssistantMode;
}
