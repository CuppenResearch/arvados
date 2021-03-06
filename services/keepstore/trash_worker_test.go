package main

import (
	"container/list"
	"testing"
	"time"
)

type TrashWorkerTestData struct {
	Locator1    string
	Block1      []byte
	BlockMtime1 int64

	Locator2    string
	Block2      []byte
	BlockMtime2 int64

	CreateData       bool
	CreateInVolume1  bool
	UseDelayToCreate bool

	UseTrashLifeTime bool

	DeleteLocator string

	ExpectLocator1 bool
	ExpectLocator2 bool
}

/* Delete block that does not exist in any of the keep volumes.
   Expect no errors.
*/
func TestTrashWorkerIntegration_GetNonExistingLocator(t *testing.T) {
	testData := TrashWorkerTestData{
		Locator1: "5d41402abc4b2a76b9719d911017c592",
		Block1:   []byte("hello"),

		Locator2: "5d41402abc4b2a76b9719d911017c592",
		Block2:   []byte("hello"),

		CreateData: false,

		DeleteLocator: "5d41402abc4b2a76b9719d911017c592",

		ExpectLocator1: false,
		ExpectLocator2: false,
	}
	performTrashWorkerTest(testData, t)
}

/* Delete a block that exists on volume 1 of the keep servers.
   Expect the second locator in volume 2 to be unaffected.
*/
func TestTrashWorkerIntegration_LocatorInVolume1(t *testing.T) {
	testData := TrashWorkerTestData{
		Locator1: TEST_HASH,
		Block1:   TEST_BLOCK,

		Locator2: TEST_HASH_2,
		Block2:   TEST_BLOCK_2,

		CreateData: true,

		DeleteLocator: TEST_HASH, // first locator

		ExpectLocator1: false,
		ExpectLocator2: true,
	}
	performTrashWorkerTest(testData, t)
}

/* Delete a block that exists on volume 2 of the keep servers.
   Expect the first locator in volume 1 to be unaffected.
*/
func TestTrashWorkerIntegration_LocatorInVolume2(t *testing.T) {
	testData := TrashWorkerTestData{
		Locator1: TEST_HASH,
		Block1:   TEST_BLOCK,

		Locator2: TEST_HASH_2,
		Block2:   TEST_BLOCK_2,

		CreateData: true,

		DeleteLocator: TEST_HASH_2, // locator 2

		ExpectLocator1: true,
		ExpectLocator2: false,
	}
	performTrashWorkerTest(testData, t)
}

/* Delete a block with matching mtime for locator in both volumes.
   Expect locator to be deleted from both volumes.
*/
func TestTrashWorkerIntegration_LocatorInBothVolumes(t *testing.T) {
	testData := TrashWorkerTestData{
		Locator1: TEST_HASH,
		Block1:   TEST_BLOCK,

		Locator2: TEST_HASH,
		Block2:   TEST_BLOCK,

		CreateData: true,

		DeleteLocator: TEST_HASH,

		ExpectLocator1: false,
		ExpectLocator2: false,
	}
	performTrashWorkerTest(testData, t)
}

/* Same locator with different Mtimes exists in both volumes.
   Delete the second and expect the first to be still around.
*/
func TestTrashWorkerIntegration_MtimeMatchesForLocator1ButNotForLocator2(t *testing.T) {
	testData := TrashWorkerTestData{
		Locator1: TEST_HASH,
		Block1:   TEST_BLOCK,

		Locator2: TEST_HASH,
		Block2:   TEST_BLOCK,

		CreateData:       true,
		UseDelayToCreate: true,

		DeleteLocator: TEST_HASH,

		ExpectLocator1: true,
		ExpectLocator2: false,
	}
	performTrashWorkerTest(testData, t)
}

/* Two different locators in volume 1.
   Delete one of them.
   Expect the other unaffected.
*/
func TestTrashWorkerIntegration_TwoDifferentLocatorsInVolume1(t *testing.T) {
	testData := TrashWorkerTestData{
		Locator1: TEST_HASH,
		Block1:   TEST_BLOCK,

		Locator2: TEST_HASH_2,
		Block2:   TEST_BLOCK_2,

		CreateData:      true,
		CreateInVolume1: true,

		DeleteLocator: TEST_HASH, // locator 1

		ExpectLocator1: false,
		ExpectLocator2: true,
	}
	performTrashWorkerTest(testData, t)
}

/* Allow default Trash Life time to be used. Thus, the newly created block
   will not be deleted becuase its Mtime is within the trash life time.
*/
func TestTrashWorkerIntegration_SameLocatorInTwoVolumesWithDefaultTrashLifeTime(t *testing.T) {
	testData := TrashWorkerTestData{
		Locator1: TEST_HASH,
		Block1:   TEST_BLOCK,

		Locator2: TEST_HASH_2,
		Block2:   TEST_BLOCK_2,

		CreateData:      true,
		CreateInVolume1: true,

		UseTrashLifeTime: true,

		DeleteLocator: TEST_HASH, // locator 1

		// Since trash life time is in effect, block won't be deleted.
		ExpectLocator1: true,
		ExpectLocator2: true,
	}
	performTrashWorkerTest(testData, t)
}

/* Perform the test */
func performTrashWorkerTest(testData TrashWorkerTestData, t *testing.T) {
	actual_permission_ttl := permission_ttl

	// Create Keep Volumes
	KeepVM = MakeTestVolumeManager(2)

	// Put test content
	vols := KeepVM.Volumes()
	if testData.CreateData {
		vols[0].Put(testData.Locator1, testData.Block1)
		vols[0].Put(testData.Locator1+".meta", []byte("metadata"))

		// One of the tests deletes a locator with different Mtimes in two different volumes
		if testData.UseDelayToCreate {
			time.Sleep(1 * time.Second)
		}

		if testData.CreateInVolume1 {
			vols[0].Put(testData.Locator2, testData.Block2)
			vols[0].Put(testData.Locator2+".meta", []byte("metadata"))
		} else {
			vols[1].Put(testData.Locator2, testData.Block2)
			vols[1].Put(testData.Locator2+".meta", []byte("metadata"))
		}
	}

	// Create TrashRequest for the test
	trashRequest := TrashRequest{
		Locator:    testData.DeleteLocator,
		BlockMtime: time.Now().Unix(),
	}

	// delay by permission_ttl to allow deletes to work
	time.Sleep(1 * time.Second)

	// Run trash worker and put the trashRequest on trashq
	trashList := list.New()
	trashList.PushBack(trashRequest)
	trashq = NewWorkQueue()

	// Trash worker would not delete block if its Mtime is within trash life time.
	// Hence, we will have to bypass it to allow the deletion to succeed.
	if !testData.UseTrashLifeTime {
		permission_ttl = time.Duration(1) * time.Second
	}
	go RunTrashWorker(trashq)

	trashq.ReplaceQueue(trashList)
	time.Sleep(10 * time.Millisecond) // give a moment to finish processing the list

	// Verify Locator1 to be un/deleted as expected
	data, _ := GetBlock(testData.Locator1, false)
	if testData.ExpectLocator1 {
		if len(data) == 0 {
			t.Errorf("Expected Locator1 to be still present: %s", testData.Locator1)
		}
	} else {
		if len(data) > 0 {
			t.Errorf("Expected Locator1 to be deleted: %s", testData.Locator1)
		}
	}

	// Verify Locator2 to be un/deleted as expected
	if testData.Locator1 != testData.Locator2 {
		data, _ = GetBlock(testData.Locator2, false)
		if testData.ExpectLocator2 {
			if len(data) == 0 {
				t.Errorf("Expected Locator2 to be still present: %s", testData.Locator2)
			}
		} else {
			if len(data) > 0 {
				t.Errorf("Expected Locator2 to be deleted: %s", testData.Locator2)
			}
		}
	}

	// One test used the same locator in two different volumes but with different Mtime values
	// Hence let's verify that only one volume has it and the other is deleted
	if (testData.ExpectLocator1) &&
		(testData.Locator1 == testData.Locator2) {
		locatorFoundIn := 0
		for _, volume := range KeepVM.Volumes() {
			if _, err := volume.Get(testData.Locator1); err == nil {
				locatorFoundIn = locatorFoundIn + 1
			}
		}
		if locatorFoundIn != 1 {
			t.Errorf("Expected locator to be found in only one volume after deleting. But found: %s", locatorFoundIn)
		}
	}

	// Done
	permission_ttl = actual_permission_ttl
	trashq.Close()
	KeepVM.Quit()
}
