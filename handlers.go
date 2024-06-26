package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AggregationResult struct {
	MinID  string  `json:"minid"`
	MaxID  string  `json:"maxid"`
	MinKey string  `json:"minkey"`
	MaxKey string  `json:"maxkey"`
	XAvg   float64 `json:"xavg"`
}

func upsertDocument(w http.ResponseWriter, r *http.Request) {
	var doc Document
	doc.ID = uuid.New().String()
	doc.Key = uuid.New().String()
	doc.X = rand.Intn(500000) + 1

	ctx, cancel := context.WithTimeout(context.Background(), config.UpsertContextTimeout*time.Millisecond)
	defer cancel()

	filter := bson.M{"key": doc.Key}
	opts := options.Update().SetUpsert(true)
	update := bson.M{"$set": doc}

	var err error
	for i := 1; i <= numRetries; i++ {
		_, err = collection.UpdateOne(ctx, filter, update, opts)
		if err == nil {
			incrementEventCount("upsert")
			break
		}
		trackMongoDBErrors(err)
		log.Printf("upsert error: %+v, attempt %v", err, i)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "Created _id: %s\n", doc.Key)
}

func findDocuments(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), config.FindContextTimeout*time.Millisecond)
	defer cancel()

	randomX := rand.Intn(500000) + 1

	filter := bson.M{"x": randomX}
	opts := options.Find().SetProjection(bson.M{"_id": 1}).SetLimit(5)

	var err error
	var cursor *mongo.Cursor
	for i := 1; i <= numRetries; i++ {
		cursor, err = collection.Find(ctx, filter, opts)
		if err == nil {
			incrementEventCount("find")
			break
		}
		trackMongoDBErrors(err)
		log.Printf("find error: %+v, attempt %v", err, i)
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	var count int
	for cursor.Next(ctx) {
		count++
		if count >= 1000 {
			break
		}
	}

	fmt.Fprintf(w, "Number of documents found: %d\n", count)
}

func aggSampleGroup(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(context.Background(), config.AggContextTimeout*time.Millisecond)
	defer cancel()

	// Configure the number of random values to generate
	numRandomValues := config.AggInQuerySize

	// Generate random X values
	randomXValues := make([]int, numRandomValues)
	for i := 0; i < numRandomValues; i++ {
		randomXValues[i] = rand.Intn(500000) + 1
	}

	pipeline := bson.A{
		bson.D{{"$match", bson.D{{"x", bson.D{{"$in", randomXValues}}}}}},
		bson.D{
			{"$group",
				bson.D{
					{"_id", 1},
					{"minid", bson.D{{"$min", "$_id"}}},
					{"maxid", bson.D{{"$max", "$_id"}}},
					{"minkey", bson.D{{"$min", "$key"}}},
					{"maxkey", bson.D{{"$max", "$key"}}},
					{"xavg", bson.D{{"$avg", "$x"}}},
				},
			},
		},
	}

	cursor, err := collection.Aggregate(ctx, pipeline)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer cursor.Close(ctx)

	var result AggregationResult
	if cursor.Next(ctx) {
		if err := cursor.Decode(&result); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	responseJSON, err := json.Marshal(result)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(responseJSON)
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "All good here at %s\n", time.Now().String())
}
