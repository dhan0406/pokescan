# Pokescan
With the increased popularity of Pokemon cards, I wanted to build an app where I could learn more about different kinds of Pokemon. This Expo mobile app is built with React Native and TypeScript frontend and Go in backend, using tesseract OCR to scan physical cards and it is hosted on Runpod serverless using Docker. 

User Flow Overview:
- Open Expo Go and take a photo of a physical pokemon card
- This image is cropped and saved as JPEG then POST multipart image to Runpod endpoint
- Load balancer picks it up
- Tesseract OCR reads the Pokemon name on top left of the card, calls PokeAPI/checks cache
- When a match is found, result: pokedex screen with details, stats, evolutions



https://github.com/user-attachments/assets/c7337289-c792-4b90-b4c6-6b5055f517ba



