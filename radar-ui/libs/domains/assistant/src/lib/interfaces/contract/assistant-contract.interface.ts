import { AssistantRole } from '../assistant-message.interface';

/**
 * The conversation is sent whole on every request: the server keeps no chat
 * state of its own.
 */
export interface AssistantChatMessageRequest {
    role: AssistantRole;
    content: string;
}

export interface AssistantChatRequest {
    integration_id: string;
    conversation: AssistantChatMessageRequest[];
    event_id?: string;
}

export interface AssistantToolActivityResponse {
    name: string;
    phase: string;
    error?: string;
}

export interface AssistantDoneResponse {
    stop_reason: string;
    error?: string;
    iterations?: number;
}

/**
 * One chunk of an answer. Exactly one field is set, mirroring the proto oneof.
 */
export interface AssistantChatChunk {
    delta?: string;
    tool_activity?: AssistantToolActivityResponse;
    done?: AssistantDoneResponse;
}

/**
 * grpc-gateway wraps every streamed message in a result envelope, and reports
 * a failure of the RPC itself in an error one.
 */
export interface AssistantChatStreamEnvelope {
    result?: AssistantChatChunk;
    error?: { message?: string; code?: number };
}
