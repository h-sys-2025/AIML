import sys
import argparse
import ollama

def run_agent():
    # 1. Parse command line arguments to match your CLI signature
    parser = argparse.ArgumentParser(description="AIML Agent CLI")
    parser.add_argument("-model", type=str, required=True, help="Ollama model name")
    parser.add_argument("-verbose", action="store_true", help="Enable verbose output")
    parser.add_argument("-turns", type=int, default=10, help="Maximum number of turns")
    parser.add_argument("-speak", action="store_true", help="Enable text-to-speech (speak mode)")
    args = parser.parse_args()

    # 2. Define the base system prompts
    base_system = "You are a helpful assistant."
    nothink_modifier = "/no_think Provide answers directly without any chain-of-thought reasoning."
    
    # State tracking
    think_mode = False  # Started as False per your user logic
    chat_history = []
    
    print(f"🤖 AIML Agent — model: {args.model} @ http://localhost:11434")
    print(f"  Speak mode {'ON — AI will read answers aloud' if args.speak else 'OFF'}")
    print(f"  Think mode {'ON' if think_mode else 'OFF — AI will respond directly without reasoning blocks'}")
    print("Commands: /clear  /help  exit\n")

    turn_count = 1

    # 3. Interactive Loop
    while turn_count <= args.turns:
        try:
            user_input = input("You> ").strip()
        except (KeyboardInterrupt, EOFError):
            print("\nExiting...")
            break

        # Handle Commands
        if not user_input:
            continue
        if user_input.lower() == 'exit':
            print("Exiting...")
            break
        elif user_input == '/clear':
            chat_history = []
            turn_count = 1
            print("  ✓ Chat history cleared.")
            continue
        elif user_input == '/help':
            print("  Available commands: /clear, /help, /nothink, exit")
            continue
        elif user_input == '/nothink':
            think_mode = False
            print("  ✓ Thinking disabled — system prompt rebuilt without think blocks.\n")
            continue

        # 4. Construct System Prompt dynamically based on think state
        current_system = base_system if think_mode else f"{base_system} {nothink_modifier}"
        
        # Build the final payload array
        messages = [{"role": "system", "content": current_system}] + chat_history
        messages.append({"role": "user", "content": user_input})

        if args.verbose:
            print(f"\n🔄 Turn {turn_count}/{args.turns}...")

        # 5. Query Ollama Local Instance
        try:
            response = ollama.chat(
                model=args.model.strip(),
                messages=messages,
                options={"think": False}  
            )
            
            assistant_response = response['message']['content']
            print(f"\n{assistant_response}\n")
            
            # --- METRICS CALCULATIONS ---
            # Ollama tracks 'eval_count' (tokens generated) and 'eval_duration' (nanoseconds spent generating)
            eval_count = response.get('eval_count', 0)
            eval_duration_ns = response.get('eval_duration', 0)
            
            # Convert nanoseconds to seconds
            eval_duration_sec = eval_duration_ns / 1_000_000_000 if eval_duration_ns > 0 else 0
            
            # Compute speed (Tokens per Second)
            tokens_per_second = eval_count / eval_duration_sec if eval_duration_sec > 0 else 0
            
            # Print performance specs
            print(f"📊 [Metrics] Generated: {eval_count} tokens | Speed: {tokens_per_second:.2f} tok/sec")
            print("-" * 50 + "\n")
            # ----------------------------

            # Save history (excluding system prompt modifications)
            chat_history.append({"role": "user", "content": user_input})
            chat_history.append({"role": "assistant", "content": assistant_response})
            
            # Text-to-Speech logic placeholder if -speak is passed
            if args.speak:
                pass

            turn_count += 1

        except Exception as e:
            print(f"❌ Error communicating with Ollama: {e}\n")

    if turn_count > args.turns:
        print("Maximum conversation turns reached. Exiting.")

if __name__ == "__main__":
    run_agent()
