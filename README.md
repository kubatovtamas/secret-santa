# Secret Santa Web App

## Project Goal:
A "Secret Santa" web application for creating and managing gift exchange rooms without user accounts.

## Main Algorithm:
- [Explainer](https://www.youtube.com/watch?v=GhnCj7Fvqt0)
- Create a list of structs with two int values: "You are X" and "You gift X" for each participant.
- Randomly shuffle the list.
- Split the structs into "You are X" and "You gift X".
- Shift the "You are X" part right by one position.
- Re-combine the structs. Each participant gets a struct, assigning them someone to gift.
