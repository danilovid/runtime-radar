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
 * One tool the assistant ran, in the order it was started. It is what the
 * "assistant is calling search_docs…" indicator is built from.
 */
export interface AssistantToolActivity {
    name: string;
    isRunning: boolean;
    error?: string;
}

/**
 * One turn of the conversation.
 *
 * Extension point: when the assistant grows tools that propose a change — a
 * rule draft, for instance — the proposal belongs here as an `artifacts` field
 * alongside `tools`, rendered as a card the user confirms. Nothing in this
 * domain writes anything today, so there is no such field yet.
 */
export interface AssistantMessage {
    id: string;
    role: AssistantRole;
    content: string;
    tools: AssistantToolActivity[];
    /** Set while the answer is still being streamed. */
    isPending: boolean;
    /** Set when the answer could not be produced or was cut short. */
    error?: string;
    stopReason?: AssistantStopReason;
}
