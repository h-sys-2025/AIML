import threading
import queue
import re
import ollama
import pyttsx3
import keyboard

# Configure Ollama settings
MODEL_NAME = "jan-nano-abliterated-4b-8500-gpu:latest"  # Use a non-thinking base model or your current model
SYSTEM_PROMPT = "Respond immediately and concisely. Do not use reasoning, thoughts, or <think> tags."

# Initialize TTS engine
tts_engine = pyttsx3.init()
tts_engine.setProperty('rate', 180) # Slightly faster speaking rate for quick replies

# Global stop event for the speech worker
stop_speech_event = threading.Event()

def speak_worker(text_queue):
    """Background thread to handle text-to-speech without freezing the terminal"""
    while True:
        text = text_queue.get()
        if text is None:
            break
        
        # Split text into small clauses/sentences for better interruption responsiveness
        sentences = re.split(r'(?<=[.?!])\s+', text)
        for sentence in sentences:
            if stop_speech_event.is_set():
                break
            if sentence.strip():
                tts_engine.say(sentence)
                tts_engine.runAndWait()
                
        text_queue.task_done()

def main():
    q = queue.Queue()
    worker = threading.Thread(target=speak_worker, args=(q,), daemon=True)
    worker.start()

    print("=== Instant Local AI Assistant Initialized ===")
    print("Type your message and press Enter. Press ANY key to interrupt speaking.\n")

    conversation_history = [{'role': 'system', 'content': SYSTEM_PROMPT}]

    while True:
        try:
            # Reset the speech interruption flag for the new turn
            stop_speech_event.clear()
            
            user_input = input("You: ")
            if not user_input.strip():
                continue

            # Instantly interrupt any leftover audio when user types a new question
            stop_speech_event.set()
            tts_engine.stop()
            while not q.empty():
                try:
                    q.get_nowait()
                    q.task_done()
                except queue.Empty:
                    break

            conversation_history.append({'role': 'user', 'content': user_input})
            print("AI: Generating fast reply...", end="\r")

            # Call Ollama with parameters that turn off the thinking behavior
            response = ollama.chat(
                model=MODEL_NAME, 
                messages=conversation_history,
                options={
                    "temperature": 0.3,       # Low temp for fast, deterministic choices
                    "top_p": 0.9,
                    "num_predict": 150        # Caps response length to ensure speed
                }
            )
            
            full_response = response['message']['content']
            
            # Clean out any accidental thinking blocks just in case
            clean_response = re.sub(r'<think>.*?</think>', '', full_response, flags=re.DOTALL).strip()
            
            print(f"AI: {clean_response}\n")
            
            if clean_response:
                q.put(clean_response)
                
            conversation_history.append({'role': 'assistant', 'content': clean_response})

            # Setup listener loop to catch interruptions while voice is playing
            while not q.empty() or tts_engine.isBusy():
                if keyboard.read_event(suppress=False):
                    print("\n[Interrupted]")
                    stop_speech_event.set()
                    tts_engine.stop()
                    break

        except KeyboardInterrupt:
            print("\nExiting program...")
            break

    q.put(None)
    worker.join()

if __name__ == "__main__":
    main()