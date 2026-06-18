import ollama

client = ollama.Client()

response = client.chat(
    model='jan-nano-abliterated-4b-8500-gpu:latest',  # or deepseek-r1
    messages=[
        {
            'role': 'user',
            'content': 'Write a short python function to reverse a string.'
        }
    ],
    # The native flag to completely turn off the thinking process
    think=False 
)

print(response['message']['content'])
