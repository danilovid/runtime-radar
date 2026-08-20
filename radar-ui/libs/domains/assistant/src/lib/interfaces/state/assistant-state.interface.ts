import { AssistantMessage } from '../assistant-message.interface';

export interface AssistantState {
    /** Whether the chat widget is open. */
    isOpen: boolean;
    /** The AI integration answering. Empty until one is picked or defaulted. */
    integrationId: string;
    /** The conversation, oldest first. It lives for as long as the page does. */
    messages: AssistantMessage[];
    /** Whether an answer is being streamed right now. */
    isStreaming: boolean;
    /** The runtime event the conversation was opened from, if any. */
    eventId: string;
}
