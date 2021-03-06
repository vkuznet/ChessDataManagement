package main

// mongo module
//
// Copyright (c) 2019 - Valentin Kuznetsov <vkuznet AT gmail dot com>
//
// References : https://gist.github.com/boj/5412538
//              https://gist.github.com/border/3489566

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"strings"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// Record define Mongo record
type Record map[string]interface{}

// ToJSON provides string representation of Record
func (r Record) ToJSON() string {
	// create pretty JSON representation of the record
	data, _ := json.MarshalIndent(r, "", "    ")
	return string(data)
}

// ToString provides string representation of Record
func (r Record) ToString() string {
	var out []string
	for _, k := range MapKeys(r) {
		if k == "_id" {
			continue
		}
		switch v := r[k].(type) {
		case int, int64:
			out = append(out, fmt.Sprintf("%s:%d", k, v))
		case float64:
			d := int(v)
			if float64(d) == v {
				out = append(out, fmt.Sprintf("%s:%d", k, d))
			} else {
				out = append(out, fmt.Sprintf("%s:%f", k, v))
			}
		case []interface{}:
			var vals []string
			for i, val := range v {
				if i == len(v)-1 {
					vals = append(vals, fmt.Sprintf("%v", val))
				} else {
					vals = append(vals, fmt.Sprintf("%v,", val))
				}
			}
			out = append(out, fmt.Sprintf("%s:%s", k, vals))
		default:
			out = append(out, fmt.Sprintf("%s:%v", k, r[k]))
		}
	}
	return strings.Join(out, "\n")
}

// ErrorRecord provides error record
func ErrorRecord(msg, etype string, ecode int) Record {
	erec := make(Record)
	erec["error"] = html.EscapeString(msg)
	erec["type"] = html.EscapeString(etype)
	erec["code"] = ecode
	return erec
}

// GetValue function to get int value from record for given key
func GetValue(rec Record, key string) interface{} {
	var val Record
	keys := strings.Split(key, ".")
	if len(keys) > 1 {
		value, ok := rec[keys[0]]
		if !ok {
			log.Printf("Unable to find key value in Record %v, key %v\n", rec, key)
			return ""
		}
		switch v := value.(type) {
		case Record:
			val = v
		case []Record:
			if len(v) > 0 {
				val = v[0]
			} else {
				return ""
			}
		case []interface{}:
			vvv := v[0]
			if vvv != nil {
				val = vvv.(Record)
			} else {
				return ""
			}
		default:
			log.Printf("Unknown type %v, rec %v, key %v keys %v\n", fmt.Sprintf("%T", v), v, key, keys)
			return ""
		}
		if len(keys) == 2 {
			return GetValue(val, keys[1])
		}
		return GetValue(val, strings.Join(keys[1:], "."))
	}
	value := rec[key]
	return value
}

// helper function to return single entry (e.g. from a list) of given value
func singleEntry(data interface{}) interface{} {
	switch v := data.(type) {
	case []interface{}:
		return v[0]
	default:
		return v
	}
}

// GetStringValue function to get string value from record for given key
func GetStringValue(rec Record, key string) (string, error) {
	value := GetValue(rec, key)
	val := fmt.Sprintf("%v", value)
	return val, nil
}

// GetSingleStringValue function to get string value from record for given key
func GetSingleStringValue(rec Record, key string) (string, error) {
	value := singleEntry(GetValue(rec, key))
	val := fmt.Sprintf("%v", value)
	return val, nil
}

// GetIntValue function to get int value from record for given key
func GetIntValue(rec Record, key string) (int, error) {
	value := GetValue(rec, key)
	val, ok := value.(int)
	if ok {
		return val, nil
	}
	return 0, fmt.Errorf("Unable to cast value for key '%s'", key)
}

// GetInt64Value function to get int value from record for given key
func GetInt64Value(rec Record, key string) (int64, error) {
	value := GetValue(rec, key)
	out, ok := value.(int64)
	if ok {
		return out, nil
	}
	return 0, fmt.Errorf("Unable to cast value for key '%s'", key)
}

// MongoConnection defines connection to MongoDB
type MongoConnection struct {
	Session *mgo.Session
}

// Connect provides connection to MongoDB
func (m *MongoConnection) Connect() *mgo.Session {
	var err error
	if m.Session == nil {
		m.Session, err = mgo.Dial(Config.URI)
		if err != nil {
			panic(err)
		}
		//         m.Session.SetMode(mgo.Monotonic, true)
		m.Session.SetMode(mgo.Strong, true)
	}
	return m.Session.Clone()
}

// global object which holds MongoDB connection
var _Mongo MongoConnection

// MongoInsert records into MongoDB
func MongoInsert(dbname, collname string, records []Record) {
	s := _Mongo.Connect()
	defer s.Close()
	c := s.DB(dbname).C(collname)
	for _, rec := range records {
		if err := c.Insert(&rec); err != nil {
			log.Printf("Fail to insert record %v, error %v\n", rec, err)
		}
	}
}

// MongoUpsert records into MongoDB
func MongoUpsert(dbname, collname string, records []Record) error {
	s := _Mongo.Connect()
	defer s.Close()
	c := s.DB(dbname).C(collname)
	for _, rec := range records {
		dataset := rec["dataset"].(string)
		if dataset == "" {
			log.Printf("no dataset, record %v\n", rec)
			continue
		}
		spec := bson.M{"dataset": dataset}
		if _, err := c.Upsert(spec, &rec); err != nil {
			log.Printf("Fail to insert record %v, error %v\n", rec, err)
			return err
		}
	}
	return nil
}

// MongoGet records from MongoDB
func MongoGet(dbname, collname string, spec bson.M, idx, limit int) []Record {
	out := []Record{}
	s := _Mongo.Connect()
	defer s.Close()
	c := s.DB(dbname).C(collname)
	var err error
	if limit > 0 {
		err = c.Find(spec).Skip(idx).Limit(limit).All(&out)
	} else {
		err = c.Find(spec).Skip(idx).All(&out)
	}
	if err != nil {
		log.Printf("Unable to get records, error %v\n", err)
	}
	return out
}

// MongoGetSorted records from MongoDB sorted by given key
func MongoGetSorted(dbname, collname string, spec bson.M, skeys []string) []Record {
	out := []Record{}
	s := _Mongo.Connect()
	defer s.Close()
	c := s.DB(dbname).C(collname)
	err := c.Find(spec).Sort(skeys...).All(&out)
	if err != nil {
		log.Printf("Unable to sort records, error %v\n", err)
		// try to fetch all unsorted data
		err = c.Find(spec).All(&out)
		if err != nil {
			log.Printf("Unable to find records, error %v\n", err)
			out = append(out, ErrorRecord(fmt.Sprintf("%v", err), MongoDBErrorName, MongoDBError))
		}
	}
	return out
}

// helper function to present in bson selected fields
func sel(q ...string) (r bson.M) {
	r = make(bson.M, len(q))
	for _, s := range q {
		r[s] = 1
	}
	return
}

// MongoUpdate inplace for given spec
func MongoUpdate(dbname, collname string, spec, newdata bson.M) {
	s := _Mongo.Connect()
	defer s.Close()
	c := s.DB(dbname).C(collname)
	err := c.Update(spec, newdata)
	if err != nil {
		log.Printf("Unable to update record, spec %v, data %v, error %v\n", spec, newdata, err)
	}
}

// MongoCount gets number records from MongoDB
func MongoCount(dbname, collname string, spec bson.M) int {
	s := _Mongo.Connect()
	defer s.Close()
	c := s.DB(dbname).C(collname)
	nrec, err := c.Find(spec).Count()
	if err != nil {
		log.Printf("Unable to count records, spec %v, error %v\n", spec, err)
	}
	return nrec
}

// MongoRemove records from MongoDB
func MongoRemove(dbname, collname string, spec bson.M) {
	s := _Mongo.Connect()
	defer s.Close()
	c := s.DB(dbname).C(collname)
	_, err := c.RemoveAll(spec)
	if err != nil && err != mgo.ErrNotFound {
		log.Printf("Unable to remove records, spec %v, error %v\n", spec, err)
	}
}
