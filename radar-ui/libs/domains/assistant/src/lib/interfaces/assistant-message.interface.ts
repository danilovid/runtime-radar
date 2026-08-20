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
    END_TURN = 'end_turn',
    ERROR = 'error',
    MAX_ITERATIONS = 'max_iterations'
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
 *
 * Extension point: when the assistant grows tools that propose a change — a
 * rule draft, for instance — the proposal belongs here as an `artifacts` field
 * alongside `tools`, rendered as a card the user confirms.
 */
export interface AssistantMessage {
    id: string;
    role: AssistantRole;
    content: string;
    tools: AssistantToolActivity[];
    attachments: AssistantAttachment[];
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
}
